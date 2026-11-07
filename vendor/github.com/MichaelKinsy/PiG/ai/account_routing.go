package ai

// Request-local account routing across the stored accounts of one provider: model-plan gates,
// OMP-compatible persisted blocks, a same-account OAuth refresh retry, and replay-safe fallback
// that never lets a partial response escape twice.
//
// Ports packages/coding-agent/src/core/account-pool.ts. The router is transport-agnostic: the
// caller supplies an attempt that opens one try for one account, so chat streams, image
// generation and classification share the same selection, blocking and fallback policy.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AccountRotation is one account handoff, as sessions and the JSON/RPC event protocols report it.
type AccountRotation struct {
	ProviderID     string  `json:"providerId"`
	FromAccountKey *string `json:"fromAccountKey"`
	ToAccountKey   *string `json:"toAccountKey"`
	// Reason is "quota", "policy" or "auth".
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	SessionID string `json:"sessionId,omitempty"`
}

// AccountRouteRequest is what one request tells the router about itself.
type AccountRouteRequest struct {
	// SessionID pins a session's successes per model within the router's plan priority.
	SessionID string
	// APIKeySet reports that the request carries an explicit key. An explicit key is the caller's
	// account decision: routing is bypassed so it is never rotated past.
	APIKeySet bool
}

// AccountSelection is the stored account one attempt resolves. AccountKeySet is always true for
// a routed attempt: an explicit nil key addresses the identity-less account, which is not the
// provider's selected one.
type AccountSelection struct {
	AccountKey    *string
	AccountKeySet bool
}

// AccountObserve receives the provider response of the current attempt, so a rejection can be
// classified by status and Retry-After. A nil observer records nothing.
type AccountObserve func(ProviderResponse)

// AccountAttemptStream opens one chat attempt for one account.
type AccountAttemptStream func(ctx context.Context, selection AccountSelection, observe AccountObserve, forceOAuthRefresh bool) (*AssistantMessageEventStream, error)

// AccountAttemptImages opens one image-generation attempt for one account.
type AccountAttemptImages func(ctx context.Context, selection AccountSelection, observe AccountObserve, forceOAuthRefresh bool) (AssistantImages, error)

// AccountAttemptClassify opens one classification attempt for one account.
type AccountAttemptClassify func(ctx context.Context, selection AccountSelection, observe AccountObserve, forceOAuthRefresh bool) (ClassifierResult, error)

// accountKeyValue is a comparable projection of a nullable account key.
type accountKeyValue struct {
	key string
	set bool
}

func accountKeyOf(accountKey *string) accountKeyValue {
	if accountKey == nil {
		return accountKeyValue{}
	}
	return accountKeyValue{key: *accountKey, set: true}
}

// AccountRouter routes independent requests without changing the provider's persistent selected
// account. A store without the account methods yields a router that never routes.
type AccountRouter struct {
	credentials AccountCredentialStore
	now         func() time.Time

	mu              sync.Mutex
	activeRoutes    map[accountRouteIdentity]map[uint64]*string
	confirmedRoutes map[accountRouteIdentity]*string
	routeSequence   uint64
	// affinity is provider → session → model → the account key that session's last success used.
	affinity map[string]map[string]map[string]*string
	// localBlocks holds this run's blocks by provider → account → scope, so an account stays
	// retired even when the store cannot persist the block.
	localBlocks map[string]map[accountKeyValue]map[string]int64
	// listeners observe handoffs.
	listeners []*accountRotationListener
}

// NewAccountRouter builds a router over store. A nil or single-account store never routes.
func NewAccountRouter(store CredentialStore) *AccountRouter {
	router := &AccountRouter{now: time.Now, affinity: map[string]map[string]map[string]*string{}, localBlocks: map[string]map[accountKeyValue]map[string]int64{}}
	if store != nil {
		if accounts, ok := store.(AccountCredentialStore); ok {
			router.credentials = accounts
		}
	}
	return router
}

// Pooled reports whether the store holds several accounts per provider.
func (r *AccountRouter) Pooled() bool { return r != nil && r.credentials != nil }

// SetClock replaces the router's clock (tests).
func (r *AccountRouter) SetClock(now func() time.Time) {
	if now != nil {
		r.now = now
	}
}

// OnRotation subscribes to account handoffs and returns the detach function.
func (r *AccountRouter) OnRotation(listener func(AccountRotation)) func() {
	if r == nil || listener == nil {
		return func() {}
	}
	detach := &accountRotationListener{listener: listener}
	r.mu.Lock()
	r.listeners = append(r.listeners, detach)
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		r.listeners = slices.DeleteFunc(r.listeners, func(existing *accountRotationListener) bool { return existing == detach })
		r.mu.Unlock()
	}
}

// ClearProvider drops this runtime's successful-request affinity for a provider, so the next
// request uses the selected account within the router's plan priority.
func (r *AccountRouter) ClearProvider(providerID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.affinity, providerID)
	for identity := range r.confirmedRoutes {
		if identity.provider == providerID {
			delete(r.confirmedRoutes, identity)
		}
	}
}

// PeekAccountKey reports the account this provider's recent requests were pinned to, for
// read-only display. present is false when the session has no pin for the provider, session and
// model; a present nil key is the identity-less account.
func (r *AccountRouter) PeekAccountKey(providerID, sessionID, modelID string) (*string, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	sessions := r.affinity[providerID]
	if sessions == nil {
		return nil, false
	}
	if sessionID == "" {
		for _, models := range sessions {
			for _, accountKey := range models {
				return accountKey, true
			}
		}
		return nil, false
	}
	models := sessions[sessionID]
	if models == nil {
		return nil, false
	}
	if modelID != "" {
		accountKey, ok := models[modelID]
		return accountKey, ok
	}
	for _, accountKey := range models {
		return accountKey, true
	}
	return nil, false
}

func (r *AccountRouter) pin(providerID, modelID string, accountKey *string, request AccountRouteRequest) {
	if request.SessionID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	sessions := r.affinity[providerID]
	if sessions == nil {
		sessions = map[string]map[string]*string{}
		r.affinity[providerID] = sessions
	}
	models := sessions[request.SessionID]
	if models == nil {
		models = map[string]*string{}
		sessions[request.SessionID] = models
	}
	models[modelID] = accountKey
	r.confirmRouteLocked(accountRouteIdentity{providerID, request.SessionID, modelID}, accountKey)
}

func (r *AccountRouter) reportRotation(providerID, modelID string, failed accountFailure, next *CredentialAccountInfo, request AccountRouteRequest) {
	reason := accountFailureReason(failed.scope)
	label := "another stored account"
	if next.Label != "" {
		label = next.Label
	} else if next.AccountKey != nil {
		label = *next.AccountKey
	}
	r.mu.Lock()
	listeners := slices.Clone(r.listeners)
	r.mu.Unlock()
	for _, subscription := range listeners {
		subscription.listener(AccountRotation{
			ProviderID:     providerID,
			FromAccountKey: failed.account.AccountKey,
			ToAccountKey:   next.AccountKey,
			Reason:         reason,
			Message:        fmt.Sprintf("Switched %s account to %s after a %s rejection.", providerID, label, reason),
			SessionID:      request.SessionID,
		})
	}
}

// accountFailureReason maps a block scope to the reported rotation reason.
func accountFailureReason(scope string) string {
	switch {
	case scope == "auth":
		return "auth"
	case scope == "account-policy" || strings.HasPrefix(scope, "model-policy:"):
		return "policy"
	}
	return "quota"
}

// accountRotationListener is one rotation subscription, kept as a pointer so a detach compares
// identity rather than function values.
type accountRotationListener struct {
	listener func(AccountRotation)
}

// accountFailure is an account this request gave up on, and why.
type accountFailure struct {
	account *CredentialAccountInfo
	scope   string
}

// Candidates returns the accounts one request may use, best first: eligible Codex free-tier
// accounts, then other eligible accounts. Within each group, the session's pinned account
// precedes the provider's selected account, then row order. Accounts whose Codex plan
// cannot serve the model are dropped up front; accounts whose plan cannot be read follow every
// known-eligible account. It returns nil when the router does not route this request.
func (r *AccountRouter) Candidates(ctx context.Context, model AnyModel, request AccountRouteRequest) ([]*CredentialAccountInfo, error) {
	if !r.Pooled() || request.APIKeySet || model == nil {
		return nil, nil
	}
	if _, routed := AccountRequestFromContext(ctx); routed {
		// This request already carries its account decision.
		return nil, nil
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	all, err := r.credentials.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	providerID, modelID := model.ProviderID(), model.ModelID()
	accounts := make([]*CredentialAccountInfo, 0, len(all))
	for i := range all {
		if all[i].ProviderID == providerID {
			account := all[i]
			accounts = append(accounts, &account)
		}
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	pinned, pinnedSet := r.PeekAccountKey(providerID, request.SessionID, modelID)
	// Explicit selection is a preference, not permission to send an ineligible account; the sort
	// is stable, so rows keep their order within a rank.
	slices.SortStableFunc(accounts, func(left, right *CredentialAccountInfo) int {
		return accountRank(right, pinned, pinnedSet) - accountRank(left, pinned, pinnedSet)
	})
	var freeTier, eligible, unknown []*CredentialAccountInfo
	for _, account := range accounts {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		if !isCodexAccountProvider(providerID) || account.Type != CredentialOAuth {
			eligible = append(eligible, account)
			continue
		}
		credential, err := r.credentials.ReadAccount(ctx, providerID, account.AccountKey)
		if err != nil {
			return nil, err
		}
		if credential == nil {
			continue
		}
		meets := AccountMeetsModelPlan(providerID, modelID, *credential)
		switch {
		case meets == nil:
			unknown = append(unknown, account)
		case *meets:
			// Personal runtime policy: preserve paid quota when a free account can serve this model, even after a paid fallback established affinity.
			plan, _ := codexPlanClaim(credential.Access)
			if slices.Contains(strings.Split(plan, "_"), "free") {
				freeTier = append(freeTier, account)
			} else {
				eligible = append(eligible, account)
			}
		}
	}
	eligible = append(freeTier, eligible...)
	eligible = append(eligible, unknown...)
	if len(eligible) == 0 {
		return nil, fmt.Errorf("No stored account meets the plan requirement for %s/%s. Sign in with an eligible account.", providerID, modelID)
	}
	return eligible, nil
}

// accountRank scores a candidate: the session's pinned account, else the provider's selected
// account, else row order.
func accountRank(account *CredentialAccountInfo, pinned *string, pinnedSet bool) int {
	switch {
	case pinnedSet && accountKeyOf(account.AccountKey) == accountKeyOf(pinned):
		return 2
	case account.Active:
		return 1
	}
	return 0
}

// ResolveAccount reports the account a request would use, for model-scoped credential exports.
// present is false for a request the router does not route.
func (r *AccountRouter) ResolveAccount(ctx context.Context, model AnyModel, request AccountRouteRequest) (accountKey *string, present bool, err error) {
	accounts, err := r.Candidates(ctx, model, request)
	if err != nil || accounts == nil {
		return nil, false, err
	}
	var earliest *int64
	for _, account := range accounts {
		if err := context.Cause(ctx); err != nil {
			return nil, false, err
		}
		until, err := r.BlockedUntil(ctx, model, account)
		if err != nil {
			return nil, false, err
		}
		if until == nil || *until <= r.nowMillis() {
			return account.AccountKey, true, nil
		}
		earliest = earliestDeadline(earliest, *until)
	}
	return nil, false, errors.New(accountUnavailableMessage(model.ProviderID(), model.ModelID(), earliest))
}

func (r *AccountRouter) nowMillis() int64 { return r.now().UnixMilli() }

// BlockedUntil reports the latest live block deadline of one account for this request's scopes,
// read from the store each time so blocks another process wrote hold too.
func (r *AccountRouter) BlockedUntil(ctx context.Context, model AnyModel, account *CredentialAccountInfo) (*int64, error) {
	scopes := AccountRequestBlockScopes(model.ProviderID(), model.ModelID())
	stored, err := r.credentials.GetAccountBlockedUntil(ctx, model.ProviderID(), account.AccountKey, scopes)
	if err != nil {
		return nil, err
	}
	now := r.nowMillis()
	until := int64(0)
	if stored != nil && *stored > now {
		until = *stored
	}
	if local := r.localBlockedUntil(model.ProviderID(), account.AccountKey, scopes, now); local > until {
		until = local
	}
	if until == 0 {
		return nil, nil
	}
	return &until, nil
}

func (r *AccountRouter) localBlockedUntil(providerID string, accountKey *string, scopes []string, now int64) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	accounts := r.localBlocks[providerID]
	if accounts == nil {
		return 0
	}
	key := accountKeyOf(accountKey)
	blocks := accounts[key]
	if blocks == nil {
		return 0
	}
	until := int64(0)
	for _, scope := range append([]string{""}, scopes...) {
		deadline := blocks[scope]
		if deadline <= now {
			delete(blocks, scope)
			continue
		}
		if deadline > until {
			until = deadline
		}
	}
	if len(blocks) == 0 {
		delete(accounts, key)
	}
	return until
}

func (r *AccountRouter) rememberBlock(providerID string, accountKey *string, scope string, until int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	accounts := r.localBlocks[providerID]
	if accounts == nil {
		accounts = map[accountKeyValue]map[string]int64{}
		r.localBlocks[providerID] = accounts
	}
	key := accountKeyOf(accountKey)
	blocks := accounts[key]
	if blocks == nil {
		blocks = map[string]int64{}
		accounts[key] = blocks
	}
	if until > blocks[scope] {
		blocks[scope] = until
	}
}

// Fail classifies a rejected attempt, records the block it earns, and returns the block. A nil
// block means a sibling account cannot repair the failure.
func (r *AccountRouter) Fail(ctx context.Context, model AnyModel, account *CredentialAccountInfo, message string, response *ProviderResponse) *AccountFailureScope {
	if account == nil {
		// An unpooled request has no account to retire; its failure is reported, never blocked.
		return nil
	}
	status := 0
	var retryAfter *int64
	if response != nil {
		status = response.Status
		retryAfter = retryAfterDelay(response.Headers, r.now())
	}
	block := AccountFailureBlock(model.ProviderID(), model.ModelID(), message, status, retryAfter)
	if block == nil {
		return nil
	}
	r.rememberBlock(model.ProviderID(), account.AccountKey, block.Scope, block.Until)
	r.invalidateConfirmedRoutes(model.ProviderID(), account.AccountKey, block.Scope)
	if context.Cause(ctx) == nil && r.credentials != nil {
		// A store failure leaves the in-memory block in place, so this run still retires the
		// account.
		_ = r.credentials.BlockAccount(ctx, model.ProviderID(), account.AccountKey, block.Scope, block.Until)
	}
	return block
}

// ClearResetBlocks drops exactly the local scopes healed by a confirmed saved
// reset. It does not establish access or change affinity: the next real request
// must still succeed before the account is confirmed. Call after the matching
// persisted blocks have been cleared. Empty scopes is a no-op.
func (r *AccountRouter) ClearResetBlocks(providerID string, accountKey *string, scopes []string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	blocks := r.localBlocks[providerID][accountKeyOf(accountKey)]
	for _, scope := range scopes {
		delete(blocks, scope)
	}
}

// retryAfterDelay parses a Retry-After header as delay-seconds or as an HTTP date.
func retryAfterDelay(headers map[string]string, now time.Time) *int64 {
	value, ok := headers["retry-after"]
	if !ok || strings.TrimSpace(value) == "" {
		return nil
	}
	if seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
		if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			return nil
		}
		return new(int64(math.Max(0, seconds*1000)))
	}
	if when, err := http.ParseTime(strings.TrimSpace(value)); err == nil {
		return new(int64(math.Max(0, float64(when.Sub(now)/time.Millisecond))))
	}
	return nil
}

// accountUnavailableMessage reports that every eligible account is blocked, naming the soonest
// retry when it is known.
func accountUnavailableMessage(providerID, modelID string, earliest *int64) string {
	reset := ""
	if earliest != nil {
		minutes := int64(math.Max(1, math.Ceil(float64(*earliest-time.Now().UnixMilli())/60_000)))
		reset = fmt.Sprintf(" Try again in ~%d min.", minutes)
	}
	return fmt.Sprintf("All eligible accounts for %s/%s are temporarily unavailable.%s", providerID, modelID, reset)
}

func earliestDeadline(current *int64, candidate int64) *int64 {
	if current == nil || candidate < *current {
		return &candidate
	}
	return current
}

// accountErrorText is the message a failed attempt reports, with its cause chain appended, so a
// wrapped provider error still classifies.
func accountErrorText(err error) string {
	if err == nil {
		return ""
	}
	reason := err.Error()
	for cause, depth := errors.Unwrap(err), 0; cause != nil && depth < 6; cause, depth = errors.Unwrap(cause), depth+1 {
		reason += ": " + cause.Error()
	}
	return reason
}

// shouldRetryWithRefresh reports whether one rejected OAuth request deserves a forced token
// refresh on the same account before the router moves on: a provider can reject a token the
// store still considers valid, and only the same account can fix that.
func shouldRetryWithRefresh(account *CredentialAccountInfo, forceOAuthRefresh bool, response *ProviderResponse) bool {
	return !forceOAuthRefresh && account != nil && account.Type == CredentialOAuth && response != nil && response.Status == 401
}

// Router returns the account router this collection routes its requests through.
func (m *Models) Router() *AccountRouter { return m.accounts }

// ObserveOnResponse composes a routing observer under an existing response hook: the observer
// records the response first, then the caller's hook runs.
func ObserveOnResponse(existing func(context.Context, ProviderResponse, *Model) error, observe AccountObserve) func(context.Context, ProviderResponse, *Model) error {
	if observe == nil {
		return existing
	}
	return func(ctx context.Context, response ProviderResponse, responseModel *Model) error {
		observe(response)
		if existing != nil {
			return existing(ctx, response, responseModel)
		}
		return nil
	}
}

// SetStore replaces the credential store the router routes over and drops the affinity and
// blocks that belonged to the previous one. A collection that already captured this router keeps
// working: the pointer stays valid, its state follows the new store.
func (r *AccountRouter) SetStore(store CredentialStore) {
	if r == nil {
		return
	}
	var accounts AccountCredentialStore
	if store != nil {
		accounts, _ = store.(AccountCredentialStore)
	}
	r.mu.Lock()
	r.credentials = accounts
	r.affinity = map[string]map[string]map[string]*string{}
	r.activeRoutes = nil
	r.confirmedRoutes = nil
	r.localBlocks = map[string]map[accountKeyValue]map[string]int64{}
	r.mu.Unlock()
}
