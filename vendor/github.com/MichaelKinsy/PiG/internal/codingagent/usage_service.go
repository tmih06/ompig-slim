package codingagent

// Ports packages/coding-agent/src/core/provider-usage.ts: provider quota
// fetching across every stored account plus ambient auth, with the upstream
// per-(provider, endpoint, account) cache, failure cool-down, in-flight
// coalescing, waiter cancellation and credential-only refresh.

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// Upstream service constants (provider-usage.ts).
const (
	// UsageRequestTimeout is the upstream default request budget.
	UsageRequestTimeout = 10 * time.Second
	// UsageReportTTL is the upstream report TTL before refresh.
	UsageReportTTL = 5 * time.Minute
	// usageFailureCoolDown is the per-credential cool-down after a failure.
	usageFailureCoolDown = 10 * time.Second
	// UsageLastGoodRetention is how long a failed probe may serve its last good
	// report.
	UsageLastGoodRetention = 24 * time.Hour
	// usageOAuthRefreshSkew refreshes a token this close to expiry.
	usageOAuthRefreshSkew = 5 * time.Minute
	// NoConfiguredCredential is the reason marker for a provider with no stored,
	// runtime or environment credential.
	NoConfiguredCredential = "no configured credential"
)

// UsageServiceOptions configures a UsageService.
type UsageServiceOptions struct {
	// Credentials is the account-aware credential store. A store without
	// ListAccounts falls back to the single-credential List/Read surface.
	Credentials ai.CredentialStore
	// Accounts, when set, is the multi-account view of the same store.
	Accounts ai.AccountCredentialStore
	// AccountRouter owns request-local backoffs; a confirmed reset heals the
	// same scopes there as in the persisted store. Nil keeps local blocks intact.
	AccountRouter *ai.AccountRouter
	// GetAuth resolves request auth, refreshing one account's OAuth token when
	// it is near expiry. Account scope travels in the overrides so an explicit
	// identity-less account is distinguishable from "the selected account". It
	// must never change credential lifecycle.
	GetAuth func(ctx context.Context, providerID string, overrides ai.AuthResolutionOverrides) (*ai.AuthResult, error)
	// BaseURLFor reports a configured provider base URL.
	BaseURLFor func(providerID string) string
	// HTTPClient is the client every probe uses. Nil means http.DefaultClient.
	HTTPClient *http.Client
	// Logger receives probe warnings.
	Logger ai.UsageLogger
	// Providers overrides the built-in registry (tests, extension providers).
	Providers []*ai.UsageProvider
	// Now reports the current time; nil means time.Now.
	Now func() time.Time
	// Random returns a float in [0,1) for cache jitter; nil means math/rand.
	Random func() float64
}

// UsageService is the per-runtime quota service behind a usage dashboard.
type UsageService struct {
	options   UsageServiceOptions
	providers []*ai.UsageProvider

	mu       sync.Mutex
	cache    map[string]*usageCacheEntry
	inFlight map[string]*usageSharedProbe

	// resetRedeemMu guards redeemInFlight, the per-account redemption slots that
	// keep one account from spending two saved resets at once.
	resetRedeemMu  sync.Mutex
	redeemInFlight map[string]*redeemInFlight
}

// usageCacheEntry is one cached probe result.
type usageCacheEntry struct {
	report      *ai.UsageReport
	expiresAt   time.Time
	reason      string
	unsupported bool
	lastGood    *usageLastGood
	// cacheVersion records the provider's cache shape when the entry was built.
	cacheVersion int
}

// usageLastGood is the last successful report of one cache key.
type usageLastGood struct {
	report    ai.UsageReport
	fetchedAt time.Time
}

// usageProbeResult is the outcome of one credential probe.
type usageProbeResult struct {
	report *ai.UsageReport
	reason string
	// unsupported marks a fetcher that declined the credential shape, so an
	// ambient probe is not reported as a failed account.
	unsupported bool
}

// usageSharedProbe is one in-flight upstream probe shared by every caller.
type usageSharedProbe struct {
	done       chan struct{}
	result     usageProbeResult
	waiters    atomic.Int64
	settled    atomic.Bool
	cancel     context.CancelFunc
	contextErr error
}

// usageProbeTarget is what to probe for one stored account or ambient key.
type usageProbeTarget struct {
	provider   string
	impl       *ai.UsageProvider
	baseURL    string
	credential ai.UsageCredential
}

// usageProviderProbe is one provider's probe work: specific accounts, or
// ambient auth when none is stored.
type usageProviderProbe struct {
	provider string
	impl     *ai.UsageProvider
	baseURL  string
	accounts []ai.CredentialAccountInfo
}

// NewUsageService builds a quota service. A nil store probes ambient auth only.
func NewUsageService(options UsageServiceOptions) *UsageService {
	providers := options.Providers
	if providers == nil {
		providers = ai.DefaultUsageProviders()
	}
	return &UsageService{
		options:   options,
		providers: providers,
		cache:     map[string]*usageCacheEntry{},
		inFlight:  map[string]*usageSharedProbe{},
	}
}

// now reports the service clock.
func (service *UsageService) now() time.Time {
	if service.options.Now != nil {
		return service.options.Now()
	}
	return time.Now()
}

// random returns the jitter source.
func (service *UsageService) random() float64 {
	if service.options.Random != nil {
		return service.options.Random()
	}
	return defaultUsageRandom()
}

// FetchSnapshot probes configured accounts concurrently and returns the final
// snapshot in provider/account order. force bypasses freshness for user refresh.
func (service *UsageService) FetchSnapshot(ctx context.Context, force bool) (ai.UsageSnapshot, error) {
	return service.FetchSnapshotProgress(ctx, force, nil)
}

type usageSnapshotLabel struct {
	probe   *usageProviderProbe
	account *ai.CredentialAccountInfo
}

// FetchSnapshotProgress publishes completed accounts as they arrive, so an
// unavailable endpoint cannot hide healthy providers behind its timeout. Each
// push is a new snapshot in stable provider/account order. Cached accounts are
// published immediately without network when force is false. The caller owns
// push and receives no calls after this method returns.
func (service *UsageService) FetchSnapshotProgress(ctx context.Context, force bool, push func(ai.UsageSnapshot)) (ai.UsageSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := context.Cause(ctx); err != nil {
		return ai.UsageSnapshot{}, err
	}
	probes, err := service.collectProbes(ctx)
	if err != nil {
		return ai.UsageSnapshot{}, err
	}
	var labels []usageSnapshotLabel
	for _, probe := range probes {
		if len(probe.accounts) == 0 {
			labels = append(labels, usageSnapshotLabel{probe: probe})
			continue
		}
		for i := range probe.accounts {
			labels = append(labels, usageSnapshotLabel{probe: probe, account: &probe.accounts[i]})
		}
	}
	results := make([]usageProbeResult, len(labels))
	completed := make([]bool, len(labels))
	type response struct {
		index  int
		result usageProbeResult
		err    error
	}
	responses := make(chan response, len(labels))
	workCtx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	// provider-usage.ts starts all account requests before Promise.all. Keep
	// the same independence, including accounts belonging to one provider.
	for index := range labels {
		workers.Go(func() {
			if workCtx.Err() != nil {
				return
			}
			label := labels[index]
			var result usageProbeResult
			var err error
			if label.account == nil {
				result, err = service.fetchAmbient(workCtx, label.probe, force)
			} else {
				result, err = service.fetchAccount(workCtx, label.probe, *label.account, force)
			}
			responses <- response{index, result, err}
		})
	}
	defer func() { cancel(); workers.Wait() }()
	for range labels {
		select {
		case <-ctx.Done():
			return ai.UsageSnapshot{}, context.Cause(ctx)
		case response := <-responses:
			if response.err != nil {
				return ai.UsageSnapshot{}, response.err
			}
			results[response.index], completed[response.index] = response.result, true
			if push != nil && response.result.reason != NoConfiguredCredential && !(labels[response.index].account == nil && response.result.unsupported) {
				push(service.usageSnapshot(labels, results, completed))
			}
		}
	}
	return service.usageSnapshot(labels, results, completed), nil
}

func (service *UsageService) usageSnapshot(labels []usageSnapshotLabel, results []usageProbeResult, completed []bool) ai.UsageSnapshot {
	snapshot := ai.UsageSnapshot{Reports: make([]ai.UsageReport, 0, len(results))}
	for index := range results {
		if !completed[index] {
			continue
		}
		result := results[index]
		if result.report != nil {
			snapshot.Reports = append(snapshot.Reports, *result.report)
			continue
		}
		// A provider nothing is configured for was never asked; reporting it would
		// invent a failure.
		if result.reason == NoConfiguredCredential {
			continue
		}
		entry := labels[index]
		// An ambient key the endpoint does not accept is not a failed account.
		if entry.account == nil && result.unsupported {
			continue
		}
		reason := result.reason
		if reason == "" {
			reason = "no usage reported"
		}
		unavailable := ai.UsageUnavailableAccount{Provider: entry.probe.provider, Reason: reason}
		if entry.account == nil {
			unavailable.Label = entry.probe.provider + " (environment)"
			unavailable.Ambient = true
		} else {
			accountKey := entry.account.AccountKey
			if accountKey != nil {
				copied := *accountKey
				unavailable.AccountKey = &copied
			}
			unavailable.Label = service.unavailableLabel(entry.probe.provider, entry.account)
		}
		snapshot.UnavailableAccounts = append(snapshot.UnavailableAccounts, unavailable)
	}
	return snapshot
}

// ActiveAccount reads the identity of the account a provider currently uses,
// without selecting or refreshing anything.
func (service *UsageService) ActiveAccount(ctx context.Context, providerID string, accountKey ...*string) (ai.UsageActiveAccount, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	var key *string
	if len(accountKey) > 0 {
		// One argument names the exact account; a nil key is an identity-less
		// account, not the store's selection.
		key = accountKey[0]
		if service.options.Accounts == nil {
			return ai.UsageActiveAccount{}, false
		}
		credential, err := service.options.Accounts.ReadAccount(ctx, providerID, key)
		if err != nil || credential == nil {
			return ai.UsageActiveAccount{}, false
		}
		return service.activeAccount(credential, key)
	}
	// No argument asks for the store's selected account.
	if service.options.Credentials == nil {
		return ai.UsageActiveAccount{}, false
	}
	credential, err := service.options.Credentials.Read(ctx, providerID)
	if err != nil || credential == nil {
		return ai.UsageActiveAccount{}, false
	}
	// Read resolves the selection, which may be a named account: report its key
	// when the store can describe it.
	if service.options.Accounts != nil {
		if accounts, err := service.options.Accounts.ListAccounts(ctx); err == nil {
			for _, account := range accounts {
				if account.ProviderID != providerID || !account.Active {
					continue
				}
				key = account.AccountKey
				break
			}
		}
	}
	return service.activeAccount(credential, key)
}

// activeAccount renders the non-secret identity of one credential.
func (service *UsageService) activeAccount(credential *ai.Credential, key *string) (ai.UsageActiveAccount, bool) {
	active := ai.UsageActiveAccount{AccountKey: key}
	if settings := ai.AccountCredentialSettings(*credential); settings != nil {
		active.Email = settings.Email
		active.AccountID = settings.AccountID
		active.OrgID = settings.OrgID
		active.OrgName = settings.OrgName
	}
	return active, true
}

// collectProbes lists every account of every provider with a usage endpoint.
func (service *UsageService) collectProbes(ctx context.Context) ([]*usageProviderProbe, error) {
	configured := make(map[string]*ai.UsageProvider, len(service.providers))
	order := make([]string, 0, len(service.providers))
	for _, provider := range service.providers {
		if provider == nil || provider.FetchUsage == nil {
			continue
		}
		if _, seen := configured[provider.ID]; seen {
			continue
		}
		configured[provider.ID] = provider
		order = append(order, provider.ID)
	}
	accountsByProvider := map[string][]ai.CredentialAccountInfo{}
	if service.options.Accounts != nil {
		accounts, err := service.options.Accounts.ListAccounts(ctx)
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			if _, ok := configured[account.ProviderID]; !ok {
				continue
			}
			accountsByProvider[account.ProviderID] = append(accountsByProvider[account.ProviderID], account)
		}
	} else if service.options.Credentials != nil {
		// A single-account store reports one credential per provider through
		// List; address it with a nil key.
		entries, err := service.options.Credentials.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if _, ok := configured[entry.ProviderID]; !ok {
				continue
			}
			accountsByProvider[entry.ProviderID] = []ai.CredentialAccountInfo{{
				ProviderID: entry.ProviderID,
				Type:       entry.Type,
				AccountKey: nil,
				Active:     true,
			}}
		}
	}
	probes := make([]*usageProviderProbe, 0, len(order))
	for _, providerID := range order {
		impl := configured[providerID]
		probe := &usageProviderProbe{
			provider: providerID,
			impl:     impl,
			accounts: accountsByProvider[providerID],
		}
		if service.options.BaseURLFor != nil {
			probe.baseURL = service.options.BaseURLFor(providerID)
		}
		probes = append(probes, probe)
	}
	return probes, nil
}

// unavailableLabel renders the label of an account that produced no report.
func (service *UsageService) unavailableLabel(providerID string, account *ai.CredentialAccountInfo) string {
	if account == nil {
		return providerID + " (environment)"
	}
	if account.Label != "" {
		return account.Label
	}
	if credential := service.readCredentialSync(providerID, account.AccountKey); credential != nil {
		if settings := ai.AccountCredentialSettings(*credential); settings != nil {
			for _, candidate := range []string{settings.Email, settings.AccountID, settings.ProjectID} {
				if candidate != "" {
					return candidate
				}
			}
		}
	}
	if account.AccountKey != nil && *account.AccountKey != "" {
		return *account.AccountKey
	}
	return "account"
}

// readCredentialSync reads an account credential for labeling, without refresh.
func (service *UsageService) readCredentialSync(providerID string, accountKey *string) *ai.Credential {
	credential, err := service.readCredentialFor(context.Background(), providerID, accountKey)
	if err != nil {
		return nil
	}
	return credential
}

// readCredentialFor reads one account or the store's active credential.
func (service *UsageService) readCredentialFor(ctx context.Context, providerID string, accountKey *string) (*ai.Credential, error) {
	if service.options.Accounts != nil {
		return service.options.Accounts.ReadAccount(ctx, providerID, accountKey)
	}
	if service.options.Credentials == nil {
		return nil, nil
	}
	return service.options.Credentials.Read(ctx, providerID)
}

// fetchAccount probes one stored account, refreshing only that account's OAuth
// token when it is close to expiry.
func (service *UsageService) fetchAccount(ctx context.Context, probe *usageProviderProbe, account ai.CredentialAccountInfo, force bool) (usageProbeResult, error) {
	credential, err := service.readCredentialFor(ctx, probe.provider, account.AccountKey)
	if err != nil {
		if ctx.Err() != nil {
			return usageProbeResult{}, context.Cause(ctx)
		}
		return usageProbeResult{reason: err.Error()}, nil
	}
	if credential == nil {
		return usageProbeResult{reason: "credential is no longer stored"}, nil
	}
	target := usageProbeTarget{
		provider:   probe.provider,
		impl:       probe.impl,
		baseURL:    probe.baseURL,
		credential: ai.UsageCredentialFromStored(credential),
	}
	key := usageCacheKey(target, "store:"+usageAccountKeyText(account.AccountKey)+":"+ai.UsageCacheIdentity(target.credential))
	// Quota freshness does not depend on token freshness. A cached report
	// must not wait for OAuth renewal that is only needed for a network probe.
	if !force {
		if cached, ok := service.cachedUsage(key, target.impl.CacheVersion); ok {
			return usageAccountResult(cached, account.AccountKey), nil
		}
	}
	if credential.Type == ai.CredentialOAuth && credential.HasExpires() &&
		service.now().Add(usageOAuthRefreshSkew).UnixMilli() >= credential.Expires {
		// Advisory: a failed refresh keeps the current token in play, and this
		// path never mutates credential lifecycle. Scope is the exact account.
		if service.options.GetAuth != nil {
			_, _ = service.options.GetAuth(ctx, probe.provider, accountOverrides(account.AccountKey))
			if refreshed, readErr := service.readCredentialFor(ctx, probe.provider, account.AccountKey); readErr == nil && refreshed != nil {
				credential = refreshed
			}
		}
	}
	target.credential = ai.UsageCredentialFromStored(credential)
	key = usageCacheKey(target, "store:"+usageAccountKeyText(account.AccountKey)+":"+ai.UsageCacheIdentity(target.credential))
	result, err := service.fetchCached(ctx, key, target, force)
	return usageAccountResult(result, account.AccountKey), err
}

func usageAccountResult(result usageProbeResult, accountKey *string) usageProbeResult {
	if result.report != nil && accountKey != nil {
		report := usageWithAccountKey(*result.report, *accountKey)
		result.report = &report
	}
	return result
}

// fetchAmbient probes the environment/runtime key when no account is stored.
func (service *UsageService) fetchAmbient(ctx context.Context, probe *usageProviderProbe, force bool) (usageProbeResult, error) {
	if service.options.GetAuth == nil {
		return usageProbeResult{reason: NoConfiguredCredential}, nil
	}
	resolution, err := service.options.GetAuth(ctx, probe.provider, ai.AuthResolutionOverrides{})
	if err != nil || resolution == nil || resolution.Auth.APIKey == "" {
		if ctx.Err() != nil {
			return usageProbeResult{}, context.Cause(ctx)
		}
		return usageProbeResult{reason: NoConfiguredCredential}, nil
	}
	target := usageProbeTarget{
		provider:   probe.provider,
		impl:       probe.impl,
		baseURL:    probe.baseURL,
		credential: ai.UsageCredential{Type: ai.CredentialAPIKey, APIKey: resolution.Auth.APIKey},
	}
	key := usageCacheKey(target, "ambient:"+ai.UsageCacheIdentity(target.credential))
	return service.fetchCached(ctx, key, target, force)
}

// accountOverrides scopes auth resolution to one stored account, preserving an
// identity-less account as an explicit nil key.
func accountOverrides(accountKey *string) ai.AuthResolutionOverrides {
	return ai.WithAccountKey(ai.AuthResolutionOverrides{}, accountKey)
}

// usageAccountKeyText renders a nil key as the empty string.
func usageAccountKeyText(key *string) string {
	if key == nil {
		return ""
	}
	return *key
}

// usageCacheKey is provider, endpoint and account identity.
func usageCacheKey(target usageProbeTarget, identity string) string {
	return target.provider + "\x00" + target.baseURL + "\x00" + identity
}

// usageWithAccountKey stamps the store account key on a report.
func usageWithAccountKey(report ai.UsageReport, accountKey string) ai.UsageReport {
	if current, ok := report.Metadata["accountKey"].(string); ok && current == accountKey {
		return report
	}
	metadata := make(map[string]any, len(report.Metadata)+1)
	for key, value := range report.Metadata {
		metadata[key] = value
	}
	metadata["accountKey"] = accountKey
	report.Metadata = metadata
	return report
}

// fetchCached serves a fresh cached report, or shares one upstream probe with
// every other caller refreshing the same account. force only ignores freshness;
// the cached last-good report and its retention survive so a failed refresh can
// still render the last known quota.
func (service *UsageService) fetchCached(ctx context.Context, key string, target usageProbeTarget, force bool) (usageProbeResult, error) {
	if err := context.Cause(ctx); err != nil {
		return usageProbeResult{}, err
	}
	cacheVersion := target.impl.CacheVersion
	if !force {
		if cached, ok := service.cachedUsage(key, cacheVersion); ok {
			return cached, nil
		}
	}
	shared := service.sharedProbe(key, target, cacheVersion, force)
	defer func() {
		// Only the last interested caller cancels upstream work: an aborted
		// dashboard must not poison a refresh another caller is awaiting.
		service.mu.Lock()
		defer service.mu.Unlock()
		if shared.waiters.Add(-1) == 0 && !shared.settled.Load() {
			shared.contextErr = context.Canceled
			shared.cancel()
		}
	}()
	select {
	case <-shared.done:
		return shared.result, nil
	case <-ctx.Done():
		return usageProbeResult{}, context.Cause(ctx)
	}
}

// cachedUsage reads only fresh entries from the matching provider cache shape.
func (service *UsageService) cachedUsage(key string, version int) (usageProbeResult, bool) {
	service.mu.Lock()
	defer service.mu.Unlock()
	cached := service.cache[key]
	if cached == nil || cached.cacheVersion != version || !cached.expiresAt.After(service.now()) {
		return usageProbeResult{}, false
	}
	return usageProbeResult{report: cached.report, reason: cached.reason, unsupported: cached.unsupported}, true
}

// sharedProbe returns the in-flight probe for a key, starting one when needed.
func (service *UsageService) sharedProbe(key string, target usageProbeTarget, cacheVersion int, force bool) *usageSharedProbe {
	service.mu.Lock()
	defer service.mu.Unlock()
	if existing, ok := service.inFlight[key]; ok && existing.contextErr == nil {
		existing.waiters.Add(1)
		return existing
	}
	// Another request may finish between fetchCached's initial lookup and
	// admission here. Recheck under the same lock that starts a shared probe.
	if cached := service.cache[key]; !force && cached != nil && cached.cacheVersion == cacheVersion && cached.expiresAt.After(service.now()) {
		done := make(chan struct{})
		close(done)
		ready := &usageSharedProbe{done: done, result: usageProbeResult{report: cached.report, reason: cached.reason, unsupported: cached.unsupported}}
		ready.settled.Store(true)
		ready.waiters.Store(1)
		return ready
	}
	probeCtx, cancel := context.WithCancel(context.Background())
	entry := &usageSharedProbe{done: make(chan struct{}), cancel: cancel}
	entry.waiters.Store(1)
	service.inFlight[key] = entry
	go func() {
		result := service.probe(probeCtx, key, target, cacheVersion)
		entry.result = result
		entry.settled.Store(true)
		service.mu.Lock()
		if service.inFlight[key] == entry {
			delete(service.inFlight, key)
		}
		service.mu.Unlock()
		cancel()
		close(entry.done)
	}()
	return entry
}

// probe performs one upstream fetch, caching the report or last-good fallback.
func (service *UsageService) probe(ctx context.Context, key string, target usageProbeTarget, cacheVersion int) usageProbeResult {
	params := ai.UsageFetchParams{
		Provider:   target.provider,
		Credential: target.credential,
		BaseURL:    target.baseURL,
	}
	if !target.impl.SupportsCredential(params) {
		kind := "API key"
		if target.credential.Type == ai.CredentialOAuth {
			kind = "OAuth"
		}
		reason := "does not support " + kind + " credentials"
		service.store(key, &usageCacheEntry{
			expiresAt:    service.now().Add(usageFailureCoolDown),
			reason:       reason,
			unsupported:  true,
			cacheVersion: cacheVersion,
		})
		return usageProbeResult{reason: reason, unsupported: true}
	}
	service.mu.Lock()
	var previous *usageLastGood
	if cached := service.cache[key]; cached != nil && cached.lastGood != nil {
		value := *cached.lastGood
		previous = &value
	}
	service.mu.Unlock()

	fctx := ai.UsageFetchContext{HTTPClient: service.options.HTTPClient, Logger: service.options.Logger}
	if previous != nil {
		copied := previous.report
		fctx.PreviousReport = &copied
	}
	// A hard request budget bounds a hung endpoint; the caller's context is the
	// shared probe's, which the last waiter cancels.
	requestCtx, cancel := context.WithTimeout(ctx, UsageRequestTimeout)
	defer cancel()
	report, err := target.impl.FetchUsage(requestCtx, params, fctx)
	if ctx.Err() != nil {
		return usageProbeResult{reason: context.Cause(ctx).Error()}
	}
	var reason string
	if err != nil {
		report = nil
		reason = err.Error()
		var httpErr *ai.UsageHTTPError
		if errors.As(err, &httpErr) && (httpErr.Status == 401 || httpErr.Status == 403) {
			// A rejected credential must not keep rendering stale quota as
			// healthy. Purging the cache says nothing about the credential's
			// lifecycle: the account stays stored and selected.
			service.purge(key)
			return usageProbeResult{reason: reason}
		}
	} else if report == nil {
		reason = "no usage reported"
	}
	now := service.now()
	if report != nil {
		ai.AttributeUsageReport(report, target.credential)
		ttl := UsageReportTTL + usageJitter(service.random(), UsageReportTTL)
		service.store(key, &usageCacheEntry{
			report:       report,
			expiresAt:    now.Add(ttl),
			lastGood:     &usageLastGood{report: *report, fetchedAt: now},
			cacheVersion: cacheVersion,
		})
		return usageProbeResult{report: report}
	}
	var lastGood *ai.UsageReport
	if target.impl.RetainsLastGoodOnFailure() && previous != nil && now.Sub(previous.fetchedAt) < UsageLastGoodRetention {
		copied := previous.report
		lastGood = &copied
	}
	backoff := usageFailureCoolDown
	if target.impl.FailureBackoffMs > 0 {
		backoff = time.Duration(target.impl.FailureBackoffMs) * time.Millisecond
	}
	entry := &usageCacheEntry{
		report:       lastGood,
		expiresAt:    now.Add(backoff + usageJitter(service.random(), backoff)),
		reason:       reason,
		lastGood:     previous,
		cacheVersion: cacheVersion,
	}
	service.store(key, entry)
	return usageProbeResult{report: lastGood, reason: reason}
}

// usageJitter is the ±25% spread applied to a TTL or cool-down.
func usageJitter(random float64, base time.Duration) time.Duration {
	return time.Duration((random*0.5 - 0.25) * float64(base))
}

// store caches one probe entry.
func (service *UsageService) store(key string, entry *usageCacheEntry) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.cache[key] = entry
}

// purge drops a cache entry, for a rejected credential.
func (service *UsageService) purge(key string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	delete(service.cache, key)
}

// defaultUsageRandom is the cache jitter source, matching Math.random.
func defaultUsageRandom() float64 { return rand.Float64() }

// UsageSnapshotProviders lists the provider ids this service probes, sorted.
func (service *UsageService) UsageSnapshotProviders() []string {
	out := make([]string, 0, len(service.providers))
	for _, provider := range service.providers {
		if provider == nil || provider.FetchUsage == nil {
			continue
		}
		out = append(out, provider.ID)
	}
	sort.Strings(out)
	return out
}
