package coding

// Ports the account surface of packages/coding-agent/src/core/model-runtime.ts: the stored
// accounts of every provider, the read-only identity of the account a session is routed to, the
// account handoff event, and the affinity reset an explicit login or account change performs.

import (
	"context"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

// AccountRotation is one request-local account handoff.
type AccountRotation = ai.AccountRotation

// ListCredentialAccounts lists every stored account, active first per provider. It is empty for a
// single-account store, which reports one credential per provider through ListCredentials.
func (runtime *ModelRuntime) ListCredentialAccounts(ctx context.Context) ([]ai.CredentialAccountInfo, error) {
	store := runtime.credentialAccounts()
	if store == nil {
		return []ai.CredentialAccountInfo{}, nil
	}
	return store.ListAccounts(ctx)
}

// credentialAccounts is the base credential store's multi-account surface, or nil.
func (runtime *ModelRuntime) credentialAccounts() ai.AccountCredentialStore {
	if runtime == nil || runtime.services == nil {
		return nil
	}
	return runtime.services.Registry().RegistryCredentialAccounts()
}

// SelectCredentialAccount makes one stored account the provider's active account and drops this
// runtime's request affinity, so the choice takes effect on the next request. False when the
// store cannot or did not select it.
func (runtime *ModelRuntime) SelectCredentialAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
	store := runtime.credentialAccounts()
	if store == nil {
		return false, nil
	}
	selected, err := store.SelectAccount(ctx, providerID, accountKey)
	if err != nil {
		return false, err
	}
	if selected {
		runtime.clearAccountAffinity(providerID)
		if err := runtime.refreshProviderAvailabilityAfterCredentialChange(ctx, providerID); err != nil {
			return true, err
		}
	}
	return selected, nil
}

// DeleteCredentialAccount removes one stored account, keeping the provider's others, and drops
// this runtime's request affinity for that provider.
func (runtime *ModelRuntime) DeleteCredentialAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
	store := runtime.credentialAccounts()
	if store == nil {
		return false, nil
	}
	removed, err := store.DeleteAccount(ctx, providerID, accountKey)
	if err != nil {
		return false, err
	}
	if removed {
		runtime.clearAccountAffinity(providerID)
		if err := runtime.refreshProviderAvailabilityAfterCredentialChange(ctx, providerID); err != nil {
			return true, err
		}
	}
	return removed, nil
}

// GetUsageActiveAccount reports the read-only identity of the account this provider's requests
// are routed to for a session and model. It never resolves credentials, refreshes tokens or
// changes the provider's selection; present is false when nothing is pinned.
func (runtime *ModelRuntime) GetUsageActiveAccount(ctx context.Context, providerID string, options ...UsageActiveAccountOptions) (ai.UsageActiveAccount, bool) {
	var opts UsageActiveAccountOptions
	if len(options) > 0 {
		opts = options[0]
	}
	accountKey, present := opts.AccountKey, opts.AccountKeyPresent
	if !present {
		accountKey, present = runtime.PeekAccountKey(providerID, opts.SessionID, opts.ModelID)
	}
	if !present {
		return ai.UsageActiveAccount{}, false
	}
	store := runtime.credentialAccounts()
	if store == nil {
		return ai.UsageActiveAccount{AccountKey: accountKey}, true
	}
	credential, err := store.ReadAccount(ctx, providerID, accountKey)
	if err != nil || credential == nil {
		return ai.UsageActiveAccount{AccountKey: accountKey}, true
	}
	active := ai.UsageActiveAccount{AccountKey: accountKey}
	if settings := ai.AccountCredentialSettings(*credential); settings != nil {
		active.Email = settings.Email
		active.AccountID = settings.AccountID
		active.OrgID = settings.OrgID
		active.OrgName = settings.OrgName
	}
	return active, true
}

// UsageActiveAccountOptions narrows GetUsageActiveAccount.
type UsageActiveAccountOptions struct {
	// SessionID and ModelID look up the session's pinned account.
	SessionID string
	ModelID   string
	// AccountKey, with AccountKeyPresent, names the account directly; a present nil key is the
	// identity-less account.
	AccountKey        *string
	AccountKeyPresent bool
}

// OnAccountRotation subscribes to request-local account handoffs and returns the detach
// function.
func (runtime *ModelRuntime) OnAccountRotation(listener func(AccountRotation)) func() {
	if runtime == nil || runtime.services == nil || listener == nil {
		return func() {}
	}
	return runtime.services.Registry().OnAccountRotation(listener)
}

// PeekAccountKey reports the account a session's requests are pinned to for a provider and model.
// present is false when nothing is pinned; a present nil key is the identity-less account.
func (runtime *ModelRuntime) PeekAccountKey(providerID, sessionID, modelID string) (*string, bool) {
	if runtime == nil || runtime.services == nil {
		return nil, false
	}
	return runtime.services.Registry().PeekAccountKey(providerID, sessionID, modelID)
}

// clearAccountAffinity drops this runtime's successful-request affinity for a provider.
func (runtime *ModelRuntime) clearAccountAffinity(providerID string) {
	if runtime == nil || runtime.services == nil {
		return
	}
	runtime.services.Registry().ClearAccountProvider(providerID)
}

// refreshProviderAvailabilityAfterCredentialChange rechecks one provider's auth snapshot after an
// account-level change, as upstream refreshProviderAvailability does.
func (runtime *ModelRuntime) refreshProviderAvailabilityAfterCredentialChange(ctx context.Context, providerID string) error {
	if runtime == nil || runtime.services == nil {
		return nil
	}
	state := &runtime.availability
	state.mu.Lock()
	state.refreshSeq++
	state.providerSeq[providerID]++
	seq := state.providerSeq[providerID]
	state.errorSeq++
	errorSeq := state.errorSeq
	state.mu.Unlock()
	return runtime.refreshProviderAvailability(ctx, providerID, seq, errorSeq)
}

// ResolveCredentialAccount reports the account a request for this model would use, for
// model-scoped credential exports such as `--provider` bearer-token printing.
func (runtime *ModelRuntime) ResolveCredentialAccount(ctx context.Context, model ai.AnyModel, sessionID string) (*string, bool, error) {
	router := runtime.accountRouter()
	if router == nil {
		return nil, false, nil
	}
	apiKeySet := false
	if runtime.services != nil {
		_, apiKeySet = runtime.services.Registry().RuntimeAPIKey(model.ProviderID())
	}
	return router.ResolveAccount(ctx, model, ai.AccountRouteRequest{SessionID: sessionID, APIKeySet: apiKeySet})
}

// AccountCredentialStore returns the base credential store's multi-account surface, or nil when
// the store holds one credential per provider.
func (runtime *ModelRuntime) AccountCredentialStore() ai.AccountCredentialStore {
	return runtime.credentialAccounts()
}

// UsageAccountKey reports the account this session's requests for provider are routed to, for the
// usage board's in-use marker. It reads the runtime's affinity only; present is false when the
// session has not routed a request for this provider and model, which asks the quota service for
// the store's default account.
func (s *Session) UsageAccountKey(providerID string) (*string, bool) {
	if s == nil {
		return nil, false
	}
	modelID := ""
	if model := s.Model(); model != nil && model.ProviderMeta.ProviderID == providerID {
		modelID = model.ID
	}
	return s.ModelRuntime().PeekAccountKey(providerID, s.ID(), modelID)
}

// emitAccountRotation publishes one account handoff as the Session event upstream's
// account_rotation reports (agent-session.ts:237,537). The event carries both nullable account
// keys unchanged, so an identity-less account stays distinguishable from an unknown one.
func (s *Session) emitAccountRotation(rotation ai.AccountRotation) {
	s.emitEvent(agent.AccountRotationEvent{
		ProviderID:     rotation.ProviderID,
		FromAccountKey: rotation.FromAccountKey,
		ToAccountKey:   rotation.ToAccountKey,
		Reason:         rotation.Reason,
		Message:        rotation.Message,
	})
}
