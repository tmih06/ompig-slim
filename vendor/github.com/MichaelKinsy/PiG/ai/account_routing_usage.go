package ai

import "slices"

// The usage display follows request attempts and successful routing identities. It does not infer access from a model name or subscription tier.
type accountRouteIdentity struct {
	provider, session, model string
}

func (r *AccountRouter) trackAccountAttempt(model AnyModel, account *CredentialAccountInfo, request AccountRouteRequest) func() {
	if request.SessionID == "" {
		return func() {}
	}
	identity := accountRouteIdentity{model.ProviderID(), request.SessionID, model.ModelID()}
	key := account.AccountKey
	if key != nil {
		key = new(*key)
	}
	r.mu.Lock()
	if r.activeRoutes == nil {
		r.activeRoutes = make(map[accountRouteIdentity]map[uint64]*string)
	}
	attempts := r.activeRoutes[identity]
	if attempts == nil {
		attempts = make(map[uint64]*string)
		r.activeRoutes[identity] = attempts
	}
	r.routeSequence++
	sequence := r.routeSequence
	attempts[sequence] = key
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		attempts := r.activeRoutes[identity]
		delete(attempts, sequence)
		if len(attempts) == 0 {
			delete(r.activeRoutes, identity)
		}
	}
}

// PeekRoutedAccountKey reads the latest accepted live attempt, falling back to this model's last confirmed route. Unverified attempts and rejected accounts stay hidden. The lookup performs no credential-store or network I/O.
func (r *AccountRouter) PeekRoutedAccountKey(providerID, sessionID, modelID string) (*string, bool) {
	if r == nil || providerID == "" || sessionID == "" || modelID == "" {
		return nil, false
	}
	r.mu.Lock()
	var sequence uint64
	var key *string
	present := false
	for candidate, accountKey := range r.activeRoutes[accountRouteIdentity{providerID, sessionID, modelID}] {
		if !present || candidate > sequence {
			sequence, key, present = candidate, accountKey, true
		}
	}
	confirmedKey, confirmed := r.confirmedRoutes[accountRouteIdentity{providerID, sessionID, modelID}]
	if present {
		present = confirmed && accountKeyOf(key) == accountKeyOf(confirmedKey)
	} else {
		key, present = confirmedKey, confirmed
	}
	if key != nil {
		key = new(*key)
	}
	r.mu.Unlock()
	if !present || r.localBlockedUntil(providerID, key, AccountRequestBlockScopes(providerID, modelID), r.nowMillis()) > 0 {
		return nil, false
	}
	return key, true
}

// A retry deadline makes an account probeable again; it does not confirm that a previously rejected account regained model access.
func (r *AccountRouter) invalidateConfirmedRoutes(provider string, key *string, scope string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for identity, confirmedKey := range r.confirmedRoutes {
		if identity.provider != provider || accountKeyOf(confirmedKey) != accountKeyOf(key) {
			continue
		}
		if scope == "" || slices.Contains(AccountRequestBlockScopes(provider, identity.model), scope) {
			delete(r.confirmedRoutes, identity)
		}
	}
}

func (r *AccountRouter) confirmAccountAttempt(model AnyModel, account *CredentialAccountInfo, request AccountRouteRequest) {
	if request.SessionID == "" || account == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.confirmRouteLocked(accountRouteIdentity{model.ProviderID(), request.SessionID, model.ModelID()}, account.AccountKey)
}

func (r *AccountRouter) confirmRouteLocked(identity accountRouteIdentity, key *string) {
	// Store replacement removes old attempts. Their late responses cannot confirm an account belonging to the outgoing store.
	current := false
	for _, attemptedKey := range r.activeRoutes[identity] {
		if accountKeyOf(attemptedKey) == accountKeyOf(key) {
			current = true
			break
		}
	}
	if !current {
		return
	}
	if r.confirmedRoutes == nil {
		r.confirmedRoutes = make(map[accountRouteIdentity]*string)
	}
	if key != nil {
		key = new(*key)
	}
	r.confirmedRoutes[identity] = key
}
