package ai

// GitHub Copilot account-scoped model discovery: GET {base}/models reduced by
// parseGitHubCopilotModelCatalog to the ids the account's picker can use,
// persisted as a credential-scoped roster alongside Codex's and Devin's
// (account_models.go). Upstream stores the same list on the credential as
// availableModelIds at login and on every token refresh; credentials minted
// by producers that never write it (an omp login stores a GitHub token with
// far-future expiry, so its refresh never runs) otherwise leave the picker
// unfiltered and the hub lists the whole bundled catalog — including models
// the account cannot call.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

const gitHubCopilotProviderID = "github-copilot"

// WithGitHubCopilotModelDiscovery installs the account's picker-visible roster
// on the provider, scoped by credential identity, with the bundled catalog as
// baseline. A scope with no roster yet keeps the baseline; a fetched roster
// replaces the listed chat models for that account.
func WithGitHubCopilotModelDiscovery(provider *ModelsProvider) *ModelsProvider {
	if provider == nil || provider.ID != gitHubCopilotProviderID {
		return provider
	}
	catalog := &accountModelCatalog{
		providerID: gitHubCopilotProviderID,
		scopeLabel: "live",
		endpoint:   copilotCatalogEndpoint,
		identity:   copilotModelIdentity,
		builtin:    func(modelID string) bool { return GetBuiltinModel(gitHubCopilotProviderID, modelID) != nil },
	}
	return withAccountModelDiscovery(provider, catalog, fetchGitHubCopilotModelRoster)
}

// copilotCatalogEndpoint buckets rosters by the provider's catalog base URL;
// the API base a request actually uses is resolved per credential by
// copilotModelAPIBase.
func copilotCatalogEndpoint(baseURL string) (string, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = "https://api.individual.githubcopilot.com"
	}
	return strings.TrimRight(base, "/"), nil
}

// copilotModelIdentity is a stable, non-secret roster key: the account,
// enterprise, and apiEndpoint claims when the credential carries them, and a
// digest of the refresh token — the durable account discriminator of both
// credential shapes (a short-lived Copilot API token and a stored GitHub token
// share the same refresh material). The digest, never the token, enters the
// scope id; a broker-redacted refresh marker is not account material.
func copilotModelIdentity(credential Credential) (string, bool) {
	identity := []string{}
	add := func(name, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			identity = append(identity, name, value)
		}
	}
	add("account", credential.AccountID)
	add("enterprise", credential.EnterpriseDomain)
	add("apiEndpoint", stringClaim(credential, "apiEndpoint"))
	if refresh := credential.Refresh; refresh != "" && refresh != RemoteRefreshSentinel {
		digest := sha256.Sum256([]byte(gitHubCopilotProviderID + "\x00" + refresh))
		add("refresh", hex.EncodeToString(digest[:16]))
	}
	if len(identity) == 0 {
		return "", false
	}
	encoded, _ := json.Marshal(identity)
	return string(encoded), true
}

// copilotModelAPIBase resolves the endpoint the account's model catalog lives
// on: the apiEndpoint an omp login records on the credential when present,
// else the token claim/enterprise/default resolution of getCopilotBaseURL.
func copilotModelAPIBase(credential *Credential) string {
	if endpoint := stringClaim(*credential, "apiEndpoint"); endpoint != "" {
		return strings.TrimRight(endpoint, "/")
	}
	return getCopilotBaseURL(credential.Access, credential.EnterpriseDomain)
}

// fetchGitHubCopilotModelRoster returns the bundled models whose ids the
// account's model catalog reports picker-visible — the same usable set
// upstream keeps through credential.availableModelIds, since its filter can
// only retain bundled ids. Picker ids without a bundled entry are dropped, as
// upstream's filter drops them.
func fetchGitHubCopilotModelRoster(ctx context.Context, _ string, credential *Credential) ([]*Model, string, error) {
	if credential == nil || credential.Access == "" {
		return nil, "", errors.New("GitHub Copilot model discovery requires OAuth credentials")
	}
	catalog, err := fetchGitHubCopilotModelsAtBase(ctx, copilotModelAPIBase(credential), credential.Access, copilotRetryPolicy{MaxRetries: 2, MaxElapsedMs: 5000})
	if err != nil {
		return nil, "", err
	}
	available := make(map[string]bool, len(catalog.AvailableModelIDs))
	for _, id := range catalog.AvailableModelIDs {
		available[id] = true
	}
	var models []*Model
	for _, generated := range ListModels(gitHubCopilotProviderID) {
		if available[generated.ID] {
			models = append(models, generated.ToModel())
		}
	}
	return models, "", nil
}
