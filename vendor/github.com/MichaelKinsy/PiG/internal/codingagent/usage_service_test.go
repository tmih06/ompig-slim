package codingagent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	sdkjson "github.com/MichaelKinsy/PiG/extensions/sdk/json"
)

// usageTestStore is a minimal multi-account credential store.
type usageTestStore struct {
	mu      sync.Mutex
	entries map[string][]ai.Credential
	keys    map[string][]string
}

func newUsageTestStore() *usageTestStore {
	return &usageTestStore{entries: map[string][]ai.Credential{}, keys: map[string][]string{}}
}

func (store *usageTestStore) add(providerID string, accountKey *string, credential ai.Credential) {
	store.mu.Lock()
	defer store.mu.Unlock()
	key := ""
	if accountKey != nil {
		key = *accountKey
	}
	store.entries[providerID] = append(store.entries[providerID], credential)
	store.keys[providerID] = append(store.keys[providerID], key)
}

func (store *usageTestStore) Read(ctx context.Context, providerID string) (*ai.Credential, error) {
	if err := ctx.Err(); err != nil {
		return nil, context.Cause(ctx)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entries := store.entries[providerID]
	if len(entries) == 0 {
		return nil, nil
	}
	credential := entries[0]
	return &credential, nil
}

func (store *usageTestStore) List(ctx context.Context) ([]ai.CredentialInfo, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	out := make([]ai.CredentialInfo, 0, len(store.entries))
	for providerID, entries := range store.entries {
		if len(entries) == 0 {
			continue
		}
		out = append(out, ai.CredentialInfo{ProviderID: providerID, Type: entries[0].Type})
	}
	return out, nil
}

func (store *usageTestStore) Modify(ctx context.Context, providerID string, fn func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
	return nil, errors.New("not implemented")
}

func (store *usageTestStore) Delete(ctx context.Context, providerID string) error { return nil }

func (store *usageTestStore) ReadAccount(ctx context.Context, providerID string, accountKey *string) (*ai.Credential, error) {
	if err := ctx.Err(); err != nil {
		return nil, context.Cause(ctx)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	key := ""
	if accountKey != nil {
		key = *accountKey
	}
	for index, candidate := range store.keys[providerID] {
		if candidate == key {
			credential := store.entries[providerID][index]
			return &credential, nil
		}
	}
	return nil, nil
}

func (store *usageTestStore) ListAccounts(ctx context.Context) ([]ai.CredentialAccountInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, context.Cause(ctx)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	out := make([]ai.CredentialAccountInfo, 0, 4)
	for providerID, entries := range store.entries {
		for index, credential := range entries {
			key := store.keys[providerID][index]
			info := ai.CredentialAccountInfo{ProviderID: providerID, Type: credential.Type, Active: index == 0}
			if key != "" {
				info.AccountKey = new(key)
			}
			out = append(out, info)
		}
	}
	return out, nil
}

func (store *usageTestStore) SaveAccount(ctx context.Context, providerID string, accountKey *string, credential ai.Credential) ([]ai.CredentialAccountInfo, error) {
	return nil, nil
}

func (store *usageTestStore) SelectAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
	return false, nil
}

func (store *usageTestStore) DeleteAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
	return false, nil
}

func (store *usageTestStore) ModifyAccount(ctx context.Context, providerID string, accountKey *string, fn func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
	return nil, nil
}

func (store *usageTestStore) GetAccountBlockedUntil(ctx context.Context, providerID string, accountKey *string, scopes []string) (*int64, error) {
	return nil, nil
}

func (store *usageTestStore) BlockAccount(ctx context.Context, providerID string, accountKey *string, scope string, untilMs int64) error {
	return nil
}

func (store *usageTestStore) AccountCount() (int, error) { return 0, nil }

var _ ai.AccountCredentialStore = (*usageTestStore)(nil)

func usageTestService(store ai.CredentialStore, providers ...*ai.UsageProvider) *UsageService {
	return NewUsageService(UsageServiceOptions{
		Credentials: store,
		Accounts:    store.(ai.AccountCredentialStore),
		Providers:   providers,
		Random:      func() float64 { return 0.5 },
	})
}

func usageReportFor(provider string) *ai.UsageReport {
	fraction := 0.25
	return &ai.UsageReport{
		Provider:  provider,
		FetchedAt: 1,
		Limits: []ai.UsageLimit{{
			ID:     provider + ":5h",
			Label:  "5 Hour",
			Scope:  ai.UsageScope{Provider: provider, WindowID: "5h"},
			Amount: ai.UsageAmount{UsedFraction: &fraction, Unit: ai.UsageUnitPercent},
		}},
	}
}

func TestUsageServiceCachesReportsUntilForced(t *testing.T) {
	store := newUsageTestStore()
	store.add("test", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "key"})
	var calls atomic.Int64
	provider := &ai.UsageProvider{ID: "test", FetchUsage: func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error) {
		calls.Add(1)
		return usageReportFor("test"), nil
	}}
	service := usageTestService(store, provider)
	if _, err := service.FetchSnapshot(context.Background(), false); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := service.FetchSnapshot(context.Background(), false); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
	if _, err := service.FetchSnapshot(context.Background(), true); err != nil {
		t.Fatalf("forced snapshot: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("upstream calls after force = %d, want 2", got)
	}
}

func TestUsageServiceCoalescesConcurrentRefreshes(t *testing.T) {
	store := newUsageTestStore()
	store.add("test", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "key"})
	var calls atomic.Int64
	release := make(chan struct{})
	provider := &ai.UsageProvider{ID: "test", FetchUsage: func(ctx context.Context, _ ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
		calls.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		}
		return usageReportFor("test"), nil
	}}
	service := usageTestService(store, provider)
	const callers = 4
	var wg sync.WaitGroup
	wg.Add(callers)
	errs := make(chan error, callers)
	for range callers {
		go func() {
			defer wg.Done()
			if _, err := service.FetchSnapshot(context.Background(), false); err != nil {
				errs <- err
			}
		}()
	}
	// Wait until exactly one upstream call is in flight before releasing it.
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

func TestUsageServiceCallerCancellationDoesNotAbortSharedProbe(t *testing.T) {
	store := newUsageTestStore()
	store.add("test", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "key"})
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	var calls atomic.Int64
	provider := &ai.UsageProvider{ID: "test", FetchUsage: func(ctx context.Context, _ ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
		calls.Add(1)
		once.Do(func() { close(started) })
		select {
		case <-release:
			return usageReportFor("test"), nil
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		}
	}}
	service := usageTestService(store, provider)

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := service.FetchSnapshot(firstCtx, false)
		firstDone <- err
	}()
	<-started

	secondDone := make(chan error, 1)
	go func() {
		_, err := service.FetchSnapshot(context.Background(), false)
		secondDone <- err
	}()
	// Give the second caller time to attach as a waiter.
	time.Sleep(20 * time.Millisecond)
	cancelFirst()
	if err := <-firstDone; err == nil {
		t.Fatal("the cancelled caller must report cancellation")
	}
	close(release)
	if err := <-secondDone; err != nil {
		t.Fatalf("the remaining caller must still get a report: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 (the probe is shared)", got)
	}
}

func TestUsageServicePurgesCacheOnAuthFailure(t *testing.T) {
	store := newUsageTestStore()
	store.add("test", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "key"})
	var calls atomic.Int64
	provider := &ai.UsageProvider{ID: "test", FetchUsage: func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error) {
		if calls.Add(1) == 1 {
			return usageReportFor("test"), nil
		}
		return nil, ai.NewUsageHTTPError("revoked", 401)
	}}
	service := usageTestService(store, provider)
	first, err := service.FetchSnapshot(context.Background(), false)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(first.Reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(first.Reports))
	}
	second, err := service.FetchSnapshot(context.Background(), true)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(second.Reports) != 0 {
		t.Fatalf("a rejected credential must not serve stale quota, got %d reports", len(second.Reports))
	}
	if len(second.UnavailableAccounts) != 1 || second.UnavailableAccounts[0].Reason == "" {
		t.Fatalf("unavailable = %+v", second.UnavailableAccounts)
	}
	// The purge must make an unforced read probe again instead of serving the
	// entry the auth failure dropped.
	before := calls.Load()
	if _, err := service.FetchSnapshot(context.Background(), false); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if calls.Load() == before && before < 3 {
		t.Fatal("cache entry was not purged after the auth failure")
	}
}

func TestUsageServiceServesLastGoodOnTransientFailure(t *testing.T) {
	store := newUsageTestStore()
	store.add("test", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "key"})
	var calls atomic.Int64
	provider := &ai.UsageProvider{ID: "test", FetchUsage: func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error) {
		if calls.Add(1) == 1 {
			return usageReportFor("test"), nil
		}
		return nil, errors.New("network down")
	}}
	service := usageTestService(store, provider)
	if _, err := service.FetchSnapshot(context.Background(), false); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	snapshot, err := service.FetchSnapshot(context.Background(), true)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snapshot.Reports) != 1 {
		t.Fatalf("reports = %d, want the last good report", len(snapshot.Reports))
	}
}

func TestUsageServiceIsolatesAccounts(t *testing.T) {
	store := newUsageTestStore()
	first := "acct-one"
	second := "acct-two"
	store.add("test", &first, ai.Credential{Type: ai.CredentialOAuth, Access: "token-one", AccountID: "account-one"})
	store.add("test", &second, ai.Credential{Type: ai.CredentialOAuth, Access: "token-two", AccountID: "account-two"})
	seen := make(chan string, 4)
	provider := &ai.UsageProvider{ID: "test", FetchUsage: func(_ context.Context, params ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
		seen <- params.Credential.AccessToken
		return usageReportFor("test"), nil
	}}
	service := usageTestService(store, provider)
	snapshot, err := service.FetchSnapshot(context.Background(), false)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snapshot.Reports) != 2 {
		t.Fatalf("reports = %d, want 2", len(snapshot.Reports))
	}
	close(seen)
	tokens := map[string]bool{}
	for token := range seen {
		tokens[token] = true
	}
	if !tokens["token-one"] || !tokens["token-two"] {
		t.Fatalf("probed tokens = %+v", tokens)
	}
	keys := map[string]bool{}
	for _, report := range snapshot.Reports {
		key, _ := report.Metadata["accountKey"].(string)
		keys[key] = true
	}
	if !keys[first] || !keys[second] {
		t.Fatalf("account keys = %+v", keys)
	}
}

func TestUsageServiceAmbientProbeUsesResolvedKey(t *testing.T) {
	var probed atomic.Int64
	provider := &ai.UsageProvider{ID: "test", FetchUsage: func(_ context.Context, params ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
		if params.Credential.APIKey != "env-key" {
			return nil, errors.New("wrong key: " + params.Credential.APIKey)
		}
		probed.Add(1)
		return usageReportFor("test"), nil
	}}
	service := NewUsageService(UsageServiceOptions{
		Providers: []*ai.UsageProvider{provider},
		GetAuth: func(context.Context, string, ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
			return &ai.AuthResult{Auth: ai.ModelAuth{APIKey: "env-key"}}, nil
		},
		Random: func() float64 { return 0.5 },
	})
	snapshot, err := service.FetchSnapshot(context.Background(), false)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snapshot.Reports) != 1 {
		t.Fatalf("reports = %+v", snapshot)
	}
	if probed.Load() != 1 {
		t.Fatalf("probes = %d, want 1", probed.Load())
	}
}

func TestUsageServiceOmitsUnconfiguredProviders(t *testing.T) {
	provider := &ai.UsageProvider{ID: "test", FetchUsage: func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error) {
		t.Fatal("unconfigured provider must not be probed")
		return nil, nil
	}}
	service := NewUsageService(UsageServiceOptions{
		Providers: []*ai.UsageProvider{provider},
		GetAuth: func(context.Context, string, ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
			return nil, nil
		},
	})
	snapshot, err := service.FetchSnapshot(context.Background(), false)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snapshot.Reports) != 0 || len(snapshot.UnavailableAccounts) != 0 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestUsageServiceReportsUnsupportedStoredAccountButNotAmbient(t *testing.T) {
	store := newUsageTestStore()
	store.add("test", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "key"})
	provider := &ai.UsageProvider{
		ID: "test",
		Supports: func(params ai.UsageFetchParams) bool {
			return params.Credential.Type == ai.CredentialOAuth
		},
		FetchUsage: func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error) {
			t.Fatal("an unsupported credential must not reach the fetcher")
			return nil, nil
		},
	}
	service := usageTestService(store, provider)
	snapshot, err := service.FetchSnapshot(context.Background(), false)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snapshot.UnavailableAccounts) != 1 {
		t.Fatalf("unavailable = %+v", snapshot.UnavailableAccounts)
	}
	if snapshot.UnavailableAccounts[0].Ambient {
		t.Fatal("a stored account is not ambient")
	}

	ambientProvider := &ai.UsageProvider{
		ID:       "test",
		Supports: func(params ai.UsageFetchParams) bool { return params.Credential.Type == ai.CredentialOAuth },
		FetchUsage: func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error) {
			return nil, nil
		},
	}
	ambient := NewUsageService(UsageServiceOptions{
		Providers: []*ai.UsageProvider{ambientProvider},
		GetAuth: func(context.Context, string, ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
			return &ai.AuthResult{Auth: ai.ModelAuth{APIKey: "env-key"}}, nil
		},
	})
	ambientSnapshot, err := ambient.FetchSnapshot(context.Background(), false)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(ambientSnapshot.UnavailableAccounts) != 0 {
		t.Fatalf("an ambient key the endpoint declines is not a failed account: %+v", ambientSnapshot.UnavailableAccounts)
	}
}

func TestUsageServiceActiveAccountReadsIdentityWithoutRefresh(t *testing.T) {
	store := newUsageTestStore()
	key := "acct-one"
	store.add("test", &key, ai.Credential{
		Type:      ai.CredentialOAuth,
		Access:    "token",
		AccountID: "account-one",
		Extra: map[string]sdkjson.RawMessage{
			"email": sdkjson.RawMessage(`"person@example.com"`),
			"orgId": sdkjson.RawMessage(`"org-1"`),
		},
	})
	service := usageTestService(store)
	active, ok := service.ActiveAccount(context.Background(), "test", &key)
	if !ok {
		t.Fatal("active account not found")
	}
	if active.Email != "person@example.com" || active.AccountID != "account-one" || active.OrgID != "org-1" {
		t.Fatalf("active = %+v", active)
	}
}

func TestUsageServiceActiveAccountVariadicContract(t *testing.T) {
	store := newUsageTestStore()
	named := "acct-named"
	store.add("test", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "identity-less"})
	store.add("test", &named, ai.Credential{Type: ai.CredentialOAuth, Access: "token", AccountID: "named-account"})
	service := usageTestService(store)

	// Zero args asks for the store's selection (the first, identity-less row
	// here) and reports its nil key rather than a named sibling.
	selected, ok := service.ActiveAccount(context.Background(), "test")
	if !ok {
		t.Fatal("selected account not found")
	}
	if selected.AccountKey != nil {
		t.Fatalf("selected key = %v, want nil (identity-less)", *selected.AccountKey)
	}

	// One explicit nil names the identity-less account.
	identityless, ok := service.ActiveAccount(context.Background(), "test", nil)
	if !ok {
		t.Fatal("identity-less account not found")
	}
	if identityless.AccountKey != nil || identityless.AccountID != "" {
		t.Fatalf("identity-less = %+v", identityless)
	}

	// One named key names that exact account.
	exact, ok := service.ActiveAccount(context.Background(), "test", &named)
	if !ok || exact.AccountKey == nil || *exact.AccountKey != named {
		t.Fatalf("exact = %+v ok=%v", exact, ok)
	}
	if exact.AccountID != "named-account" {
		t.Fatalf("exact account id = %q", exact.AccountID)
	}
}
