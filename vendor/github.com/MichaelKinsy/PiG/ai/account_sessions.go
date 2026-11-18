package ai

import "context"

// AccountSessionRoute identifies a sticky assignment within one provider.
type AccountSessionRoute struct {
	SessionID string                `json:"sessionId"`
	ModelID   string                `json:"modelId"`
	Usage     []AccountSessionUsage `json:"usage,omitempty"`
}

// AccountSessionStore coordinates session assignments at the credential owner.
// Stores without this capability retain selected-account routing.
type AccountSessionStore interface {
	// ResolveSessionAccount previews the next assignment without writing when reserve is false.
	// Reserving atomically reuses a valid pin or advances the shared assignment cursor.
	// A nil result means no account is currently eligible and unblocked.
	ResolveSessionAccount(context.Context, string, AccountSessionRoute, bool) (*AccountSelection, error)
	// SetSessionAccount records a fallback before its request starts, without advancing the cursor.
	SetSessionAccount(context.Context, string, AccountSessionRoute, *string) error
}

func (r *AccountRouter) sessionStore(request AccountRouteRequest) (AccountSessionStore, bool) {
	if request.SessionID == "" {
		return nil, false
	}
	var store CredentialStore = r.credentials
	for store != nil {
		if view, ok := store.(interface{ UnwrapCredentialStore() CredentialStore }); ok {
			store = view.UnwrapCredentialStore()
			continue
		}
		sessions, ok := store.(AccountSessionStore)
		return sessions, ok
	}
	return nil, false
}

func (r *AccountRouter) setSessionAccount(ctx context.Context, model AnyModel, account *CredentialAccountInfo, request AccountRouteRequest, usage []AccountSessionUsage) error {
	if store, ok := r.sessionStore(request); ok {
		route := AccountSessionRoute{SessionID: request.SessionID, ModelID: model.ModelID(), Usage: usage}
		return store.SetSessionAccount(ctx, model.ProviderID(), route, account.AccountKey)
	}
	return nil
}

type accountPlanRank int

const (
	accountPlanFree accountPlanRank = iota
	accountPlanEligible
	accountPlanUnknown
	accountPlanIneligible
)

func accountPlanPriority(providerID, modelID string, credential Credential) accountPlanRank {
	return accountPlanPriorityWithUsage(providerID, modelID, credential, nil)
}

func accountPlanPriorityWithUsage(providerID, modelID string, credential Credential, usage *AccountSessionUsage) accountPlanRank {
	usagePlan := ""
	if usage != nil {
		usagePlan = usage.Plan
	}
	meets := accountMeetsModelPlan(providerID, modelID, credential, usagePlan)
	if meets == nil {
		return accountPlanUnknown
	}
	if !*meets {
		return accountPlanIneligible
	}
	if isCodexAccountProvider(providerID) && credential.Type == CredentialOAuth {
		plan, _ := codexAccountPlan(credential)
		if normalized, ok := NormalizeCodexAccountPlan(usagePlan); ok {
			plan = normalized
		}
		if codexAccountPlanTierOf(plan) == codexAccountPlanFree {
			return accountPlanFree
		}
	}
	return accountPlanEligible
}
