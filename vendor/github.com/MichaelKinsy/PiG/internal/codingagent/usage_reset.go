package codingagent

// Ports packages/ai/src/auth/resets.ts: the per-account saved-reset inventory
// and the redeem action behind `/usage reset`.
//
// Both operations run against the stored accounts of the two providers that
// bank resets (Codex and Claude). Everything else is either not a pooled
// provider or has no reset surface, so the listing simply omits it.

import (
	"context"
	"sort"
	"sync"

	"github.com/MichaelKinsy/PiG/ai"
)

// ResetCreditProviders are the provider ids whose saved resets can be spent.
var ResetCreditProviders = []string{"openai-codex", "anthropic"}

// UsageResetCreditAccount is one stored account's live saved-reset inventory.
//
// AvailableCount is the banked inventory, which includes grants the provider
// will not let the account spend yet. RedeemableCount is the subset it permits
// now, so a caller can show "3 saved, 1 usable" instead of implying all three
// are spendable.
type UsageResetCreditAccount struct {
	ProviderID string
	// AccountKey is the store identity of the account; nil is the identity-less
	// account, which is a real account and not the provider's selection.
	AccountKey *string
	// Label is the account identity shown beside the provider.
	Label   string
	Active  bool
	Credits *ai.UsageResetCredits
	// Err reports why this account's inventory could not be read.
	Err string
}

// UsageResetTarget names the account and credit a redemption spends.
type UsageResetTarget struct {
	ProviderID string
	// AccountKey names the stored account; nil is the identity-less account.
	AccountKey *string
	// CreditID is the exact credit to spend. Empty asks the provider for its
	// next spendable credit, which is only safe when the provider pins one.
	CreditID string
	Label    string
}

// UsageResetOutcome is the result of one redemption attempt. Identity fields
// echo the resolved account so a caller that addressed it indirectly can name
// what was actually spent.
type UsageResetOutcome struct {
	ProviderID string
	AccountKey *string
	Label      string
	Ok         bool
	Code       string
	CreditID   string
	Cleared    []string
	Reason     string
}

// UsageResetQuotaService is the reset surface the `/usage reset` command needs.
// *UsageService satisfies it; the interface keeps the command testable.
type UsageResetQuotaService interface {
	// ListResetCredits reads every stored account's saved-reset inventory.
	ListResetCredits(ctx context.Context) ([]UsageResetCreditAccount, error)
	// RedeemResetCredit spends one saved reset for one stored account.
	RedeemResetCredit(ctx context.Context, target UsageResetTarget) (UsageResetOutcome, error)
}

// redeemInFlight is one account's redemption attempt. Manual and automatic
// callers share it so one account can never spend two credits at once.
type redeemInFlight struct {
	creditID string
	promise  chan struct{}
	outcome  UsageResetOutcome
	err      error
}

// ListResetCredits reads the saved-reset inventory of every stored Codex and
// Claude account concurrently, in a stable provider/account order. An account
// that fails keeps its error and is listed rather than dropped, so the user can
// see which account is unreachable.
func (service *UsageService) ListResetCredits(ctx context.Context) ([]UsageResetCreditAccount, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	accounts, err := service.resetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]UsageResetCreditAccount, len(accounts))
	var workers sync.WaitGroup
	for index := range accounts {
		workers.Go(func() {
			results[index] = service.listAccountResetCredits(ctx, accounts[index])
		})
	}
	workers.Wait()
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	return results, nil
}

// listAccountResetCredits reads one account's inventory, refreshing its token
// first when it is near expiry so a stale token is not reported as an error.
func (service *UsageService) listAccountResetCredits(ctx context.Context, account ai.CredentialAccountInfo) UsageResetCreditAccount {
	entry := UsageResetCreditAccount{
		ProviderID: account.ProviderID,
		AccountKey: copyAccountKey(account.AccountKey),
		Label:      service.resetAccountLabel(account),
		Active:     account.Active,
	}
	credential, err := service.resetCredential(ctx, account)
	if err != nil {
		entry.Err = err.Error()
		return entry
	}
	if credential == nil {
		entry.Err = "credential is no longer stored"
		return entry
	}
	if credential.Type != ai.CredentialOAuth {
		entry.Err = "saved resets need a signed-in account"
		return entry
	}
	usageCredential := ai.UsageCredentialFromStored(credential)
	probeCtx, cancel := context.WithTimeout(ctx, UsageRequestTimeout)
	defer cancel()
	fctx := ai.UsageFetchContext{HTTPClient: service.options.HTTPClient, Logger: service.options.Logger}
	baseURL := service.providerBaseURL(account.ProviderID)
	switch account.ProviderID {
	case "openai-codex":
		credits, ok := ai.ListCodexResetCredits(probeCtx, fctx, usageCredential, baseURL)
		if !ok {
			entry.Err = "failed to load saved resets"
			return entry
		}
		entry.Credits = credits
	case "anthropic":
		credits := ai.ListClaudeResetCredits(probeCtx, fctx, usageCredential, baseURL)
		if credits == nil {
			entry.Err = "failed to load saved resets"
			return entry
		}
		entry.Credits = credits
	default:
		entry.Err = "provider does not bank saved resets"
	}
	return entry
}

// resetAccounts lists the stored accounts of every reset provider, in provider
// order and then the store's own order (active first).
func (service *UsageService) resetAccounts(ctx context.Context) ([]ai.CredentialAccountInfo, error) {
	allowed := make(map[string]bool, len(ResetCreditProviders))
	for _, providerID := range ResetCreditProviders {
		allowed[providerID] = true
	}
	var out []ai.CredentialAccountInfo
	if service.options.Accounts != nil {
		accounts, err := service.options.Accounts.ListAccounts(ctx)
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			if allowed[account.ProviderID] {
				out = append(out, account)
			}
		}
	} else if service.options.Credentials != nil {
		// A single-account store reports one credential per provider, which is
		// addressed by a nil account key.
		entries, err := service.options.Credentials.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if allowed[entry.ProviderID] {
				out = append(out, ai.CredentialAccountInfo{ProviderID: entry.ProviderID, Type: entry.Type, Active: true})
			}
		}
	}
	sort.SliceStable(out, func(left, right int) bool {
		if out[left].ProviderID != out[right].ProviderID {
			return resetProviderRank(out[left].ProviderID) < resetProviderRank(out[right].ProviderID)
		}
		// The store already lists the active account first; keep that order.
		return false
	})
	return out, nil
}

// resetProviderRank orders providers as ResetCreditProviders declares them.
func resetProviderRank(providerID string) int {
	for index, candidate := range ResetCreditProviders {
		if candidate == providerID {
			return index
		}
	}
	return len(ResetCreditProviders)
}

// resetAccountLabel renders the account identity a picker row shows.
func (service *UsageService) resetAccountLabel(account ai.CredentialAccountInfo) string {
	if account.Label != "" {
		return account.Label
	}
	return service.unavailableLabel(account.ProviderID, &account)
}

// providerBaseURL reports the configured base URL of a provider.
func (service *UsageService) providerBaseURL(providerID string) string {
	if service.options.BaseURLFor == nil {
		return ""
	}
	return service.options.BaseURLFor(providerID)
}

// resetCredential reads one stored account's credential, resolving auth through
// the request path when its OAuth token is near expiry. A failed refresh keeps
// the stored token in play: the probe then reports the real auth failure.
func (service *UsageService) resetCredential(ctx context.Context, account ai.CredentialAccountInfo) (*ai.Credential, error) {
	credential, err := service.readCredentialFor(ctx, account.ProviderID, account.AccountKey)
	if err != nil {
		return nil, err
	}
	if credential == nil {
		return nil, nil
	}
	if credential.Type != ai.CredentialOAuth || !credential.HasExpires() ||
		service.now().Add(usageOAuthRefreshSkew).UnixMilli() < credential.Expires {
		return credential, nil
	}
	if service.options.GetAuth == nil {
		return credential, nil
	}
	_, _ = service.options.GetAuth(ctx, account.ProviderID, accountOverrides(account.AccountKey))
	if refreshed, readErr := service.readCredentialFor(ctx, account.ProviderID, account.AccountKey); readErr == nil && refreshed != nil {
		return refreshed, nil
	}
	return credential, nil
}

// RedeemResetCredit spends one saved reset. It re-reads the account's live
// inventory first, because a credit is a scarce, irreversible purchase: the
// offer may already have changed, been spent elsewhere, or become ineligible.
//
// One account can only redeem once at a time, and every success invalidates the
// account's cached usage report so the next dashboard or status-border refresh
// reads the freshly reset window instead of the pre-reset numbers.
func (service *UsageService) RedeemResetCredit(ctx context.Context, target UsageResetTarget) (UsageResetOutcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	outcome := UsageResetOutcome{ProviderID: target.ProviderID, AccountKey: copyAccountKey(target.AccountKey), Label: target.Label}
	switch target.ProviderID {
	case "openai-codex", "anthropic":
	default:
		outcome.Code = "unsupported_provider"
		return outcome, nil
	}
	accountKey := accountKeyText(target.AccountKey)
	service.resetRedeemMu.Lock()
	if service.redeemInFlight == nil {
		service.redeemInFlight = map[string]*redeemInFlight{}
	}
	flightKey := target.ProviderID + "\x00" + accountKey
	if existing := service.redeemInFlight[flightKey]; existing != nil {
		service.resetRedeemMu.Unlock()
		if existing.creditID != target.CreditID {
			outcome.Code = "reset_in_progress"
			return outcome, nil
		}
		select {
		case <-existing.promise:
			return existing.outcome, existing.err
		case <-ctx.Done():
			return UsageResetOutcome{}, context.Cause(ctx)
		}
	}
	flight := &redeemInFlight{creditID: target.CreditID, promise: make(chan struct{})}
	service.redeemInFlight[flightKey] = flight
	service.resetRedeemMu.Unlock()

	// Bound discovery and mutation together even when the caller has no deadline.
	redeemCtx, cancel := context.WithTimeout(ctx, 4*UsageRequestTimeout)
	defer cancel()
	result, err := service.redeemResetCredit(redeemCtx, target)
	// Closing the promise publishes both the verdict and error to joined callers.
	service.resetRedeemMu.Lock()
	flight.outcome, flight.err = result, err
	if service.redeemInFlight[flightKey] == flight {
		delete(service.redeemInFlight, flightKey)
	}
	service.resetRedeemMu.Unlock()
	close(flight.promise)
	return result, err
}

// redeemResetCredit performs the redeem under the account's in-flight slot.
func (service *UsageService) redeemResetCredit(ctx context.Context, target UsageResetTarget) (UsageResetOutcome, error) {
	outcome := UsageResetOutcome{ProviderID: target.ProviderID, AccountKey: copyAccountKey(target.AccountKey), Label: target.Label}
	var account *ai.CredentialAccountInfo
	if service.options.Accounts != nil {
		accounts, err := service.options.Accounts.ListAccounts(ctx)
		if err != nil {
			return outcome, err
		}
		for index := range accounts {
			candidate := accounts[index]
			if candidate.ProviderID == target.ProviderID && accountKeyText(candidate.AccountKey) == accountKeyText(target.AccountKey) {
				account = &candidate
				break
			}
		}
	} else {
		// A single-account store holds one credential per provider, addressed by a
		// nil key; a nil target key is therefore the only account it can name.
		if target.AccountKey == nil && service.options.Credentials != nil {
			account = &ai.CredentialAccountInfo{ProviderID: target.ProviderID, Active: true}
		}
	}
	if account == nil {
		outcome.Code = "no_account"
		return outcome, nil
	}
	if outcome.Label == "" {
		outcome.Label = service.resetAccountLabel(*account)
	}
	credential, err := service.resetCredential(ctx, *account)
	if err != nil || credential == nil {
		outcome.Code = "account_unavailable"
		return outcome, nil
	}
	if credential.Type != ai.CredentialOAuth {
		outcome.Code = "account_unavailable"
		return outcome, nil
	}
	usageCredential := ai.UsageCredentialFromStored(credential)
	fctx := ai.UsageFetchContext{HTTPClient: service.options.HTTPClient, Logger: service.options.Logger}
	baseURL := service.providerBaseURL(target.ProviderID)
	creditID := target.CreditID
	if target.ProviderID == "anthropic" {
		creditID, outcome = service.redeemClaudeReset(ctx, fctx, usageCredential, baseURL, creditID, outcome)
	} else {
		creditID, outcome = service.redeemCodexReset(ctx, fctx, usageCredential, baseURL, creditID, outcome)
	}
	outcome.CreditID = creditID
	if !outcome.Ok {
		return outcome, nil
	}
	service.clearResetBlocks(target.ProviderID, *account, outcome.Cleared)
	service.invalidateUsageAfterReset(target.ProviderID, *account)
	return outcome, nil
}

// clearResetBlocks lifts the routing blocks the freshly reset windows carried.
//
// A rejected request records the account as unavailable until the provider's
// window ends, which is exactly what a spent reset invalidates: without this,
// the account stays retired for the remainder of the old window even though its
// quota is whole again.
//
// Claude can reset only some windows, so its clear covers the windows the
// provider confirmed plus the unscoped row only when every shared window was
// covered; a partial reset keeps the blocks of the windows it left alone. Codex
// resets the account's chat meter, so its clear names that meter rather than
// every scope, and an auth or model-policy block survives.
func (service *UsageService) clearResetBlocks(providerID string, account ai.CredentialAccountInfo, cleared []string) {
	clearer, ok := service.resetBlockClearer()
	if !ok {
		return
	}
	scopes := resetBlockScopes(providerID, cleared)
	if scopes == nil {
		// Unknown coverage must never lift an independent quota wall.
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), UsageRequestTimeout)
	defer cancel()
	if err := clearer.ClearAccountBlocks(ctx, providerID, account.AccountKey, scopes); err != nil {
		// Advisory: the spend already happened; retain local blocks on store failure.
		if service.options.Logger != nil {
			service.options.Logger.Warn("Saved-reset block clear failed", map[string]any{"provider": providerID, "error": err.Error()})
		}
		return
	}
	if service.options.AccountRouter != nil {
		service.options.AccountRouter.ClearResetBlocks(providerID, account.AccountKey, scopes)
	}
}

// resetBlockClearer returns the block-clearing surface of the credential store.
func (service *UsageService) resetBlockClearer() (ai.AccountBlockClearer, bool) {
	stores := []ai.CredentialStore{service.options.Accounts, service.options.Credentials}
	for _, store := range stores {
		if store == nil {
			continue
		}
		if clearer, ok := store.(ai.AccountBlockClearer); ok {
			return clearer, true
		}
	}
	return nil, false
}

// resetBlockScopes names the block scopes a spent reset covers. A nil result
// means the provider's answer did not say what it reset, so no block is lifted.
func resetBlockScopes(providerID string, cleared []string) []string {
	windows := make([]string, 0, len(cleared))
	for _, window := range cleared {
		windows = append(windows, window)
	}
	switch providerID {
	case "anthropic":
		if len(windows) == 0 {
			return nil
		}
		scopes := append([]string{}, windows...)
		// The unscoped row is where a whole-account wall lands, so only a reset
		// that covered both shared windows may lift it.
		if len(windows) > 0 && containsAll(windows, []string{"anthropic:5h", "anthropic:7d"}) {
			scopes = append(scopes, "")
		}
		return scopes
	case "openai-codex":
		return []string{"chat", "shared"}
	}
	return nil
}

// containsAll reports whether every wanted value is present.
func containsAll(values, wanted []string) bool {
	for _, want := range wanted {
		found := false
		for _, value := range values {
			if value == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// redeemCodexReset spends a Codex credit, choosing the soonest-expiring one when
// the caller names none.
func (service *UsageService) redeemCodexReset(ctx context.Context, fctx ai.UsageFetchContext, credential ai.UsageCredential, baseURL, creditID string, outcome UsageResetOutcome) (string, UsageResetOutcome) {
	if creditID == "" {
		list, ok := ai.ListCodexResetCredits(ctx, fctx, credential, baseURL)
		if !ok {
			outcome.Code = "credit_list_failed"
			return creditID, outcome
		}
		credit := ai.PickSoonestExpiringResetCredit(list.Credits)
		if credit == nil {
			outcome.Code = "no_credit"
			return creditID, outcome
		}
		creditID = credit.ID
	}
	redeemCtx, cancel := context.WithTimeout(ctx, UsageRequestTimeout)
	defer cancel()
	consumed, err := ai.ConsumeCodexResetCredit(redeemCtx, fctx, credential, baseURL, creditID)
	if err != nil {
		outcome.Code = "network_error"
		return creditID, outcome
	}
	applyConsumeResult(&outcome, consumed)
	return creditID, outcome
}

// redeemClaudeReset spends Claude's server-selected reset. Claude names the one
// grant that may be spent, so an inventory without that pin is a refusal rather
// than an invitation to spend whichever grant looks current.
func (service *UsageService) redeemClaudeReset(ctx context.Context, fctx ai.UsageFetchContext, credential ai.UsageCredential, baseURL, creditID string, outcome UsageResetOutcome) (string, UsageResetOutcome) {
	list := ai.ListClaudeResetCredits(ctx, fctx, credential, baseURL)
	if list == nil {
		outcome.Code = "credit_list_failed"
		return creditID, outcome
	}
	selected := resetCreditByID(list, list.NextCreditID)
	if creditID != "" && creditID != list.NextCreditID {
		outcome.Code = "offer_changed"
		outcome.CreditID = creditID
		return creditID, outcome
	}
	redeemable := 0
	if list.RedeemableCount != nil {
		redeemable = *list.RedeemableCount
	}
	if list.Eligible == nil || !*list.Eligible || selected == nil || selected.Usable == nil || !*selected.Usable || redeemable < 1 {
		switch {
		case list.AvailableCount > 0:
			outcome.Code = "ineligible"
		default:
			outcome.Code = "no_credit"
		}
		outcome.Reason = list.Reason
		return creditID, outcome
	}
	creditID = selected.ID
	applyConsumeResult(&outcome, ai.ConsumeClaudeResetCredit(ctx, fctx, credential, baseURL, *selected, ""))
	return creditID, outcome
}

// applyConsumeResult copies a provider verdict onto the redeem outcome.
func applyConsumeResult(outcome *UsageResetOutcome, consumed *ai.UsageResetConsumeResult) {
	outcome.Ok = consumed.Ok
	outcome.Code = consumed.Code
	outcome.Cleared = consumed.Cleared
	outcome.Reason = consumed.Reason
}

// resetCreditByID finds one credit in an inventory.
func resetCreditByID(list *ai.UsageResetCredits, creditID string) *ai.UsageResetCreditDetail {
	if list == nil || creditID == "" {
		return nil
	}
	for index := range list.Credits {
		if list.Credits[index].ID == creditID {
			return &list.Credits[index]
		}
	}
	return nil
}

// invalidateUsageAfterReset drops the cached usage report of the account that
// just spent a reset, so the next read fetches the reset window instead of
// serving the pre-reset numbers for the rest of the TTL.
func (service *UsageService) invalidateUsageAfterReset(providerID string, account ai.CredentialAccountInfo) {
	credential, err := service.readCredentialFor(context.Background(), providerID, account.AccountKey)
	if err != nil || credential == nil {
		return
	}
	target := usageProbeTarget{
		provider:   providerID,
		baseURL:    service.providerBaseURL(providerID),
		credential: ai.UsageCredentialFromStored(credential),
	}
	service.purge(usageCacheKey(target, "store:"+accountKeyText(account.AccountKey)+":"+ai.UsageCacheIdentity(target.credential)))
}

// accountKeyText renders a nullable account key as a comparable string.
func accountKeyText(key *string) string {
	if key == nil {
		return ""
	}
	return *key
}

// copyAccountKey copies a nullable account key, preserving "set to nil" as an
// identity-less account rather than as the provider's selection.
func copyAccountKey(key *string) *string {
	if key == nil {
		return nil
	}
	copied := *key
	return &copied
}

// resetProviderLabel names a reset provider as the UI shows it.
func resetProviderLabel(providerID string) string {
	switch providerID {
	case "openai-codex":
		return "Codex"
	case "anthropic":
		return "Claude"
	}
	return providerID
}
