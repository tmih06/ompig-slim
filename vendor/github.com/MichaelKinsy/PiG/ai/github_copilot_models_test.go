package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// copilotCatalogBody builds a GET /models response from the given rows, each
// {id, picker, policy, toolCalls}; parseGitHubCopilotModelCatalog keeps the
// picker-enabled, non-disabled, tool-calling ids as the usable set.
func copilotCatalogBody(rows ...copilotCatalogRow) string {
	var entries []string
	for _, row := range rows {
		entries = append(entries, fmt.Sprintf(`{"id":%q,"model_picker_enabled":%t,"policy":{"state":%q},"capabilities":{"supports":{"tool_calls":%t}}}`, row.id, row.picker, row.policy, row.toolCalls))
	}
	return `{"data":[` + strings.Join(entries, ",") + `]}`
}

type copilotCatalogRow struct {
	id        string
	picker    bool
	policy    string
	toolCalls bool
}

// stubCopilotModelsAPI replaces the default transport so
// fetchGitHubCopilotModels's GET {base}/models is served from the fixture.
// It returns a counter of the requests the stub answered.
func stubCopilotModelsAPI(t *testing.T, serve func(request *http.Request) string) *int {
	t.Helper()
	requests := new(int)
	old := http.DefaultClient.Transport
	http.DefaultClient.Transport = codexRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		*requests++
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(serve(request))),
			Request:    request,
		}, nil
	})
	t.Cleanup(func() { http.DefaultClient.Transport = old })
	return requests
}

func copilotDiscoveryTestProvider(t *testing.T, baseURL string) *ModelsProvider {
	t.Helper()
	auth, err := BuiltinProviderAuth(gitHubCopilotProviderID)
	if err != nil {
		t.Fatal(err)
	}
	provider := &ModelsProvider{
		ID:      gitHubCopilotProviderID,
		BaseURL: baseURL,
		Auth:    auth,
		GetModels: func() ([]*Model, error) {
			return GetBuiltinModels(gitHubCopilotProviderID), nil
		},
		GetAllModels: func() ([]AnyModel, error) {
			return AnyModels(GetBuiltinModels(gitHubCopilotProviderID)), nil
		},
	}
	return WithGitHubCopilotModelDiscovery(provider)
}

// copilotOMPCredential mirrors the OAuth row an omp login stores: a GitHub
// token in both token slots, a far-future expiry (its refresh never runs) and
// no availableModelIds — the credential shape that left the picker unfiltered.
func copilotOMPCredential(token string) *Credential {
	return &Credential{
		Type:    CredentialOAuth,
		Access:  token,
		Refresh: token,
		Expires: time.Now().Add(365 * 24 * time.Hour).UnixMilli(),
		Extra: map[string]json.RawMessage{
			"apiEndpoint":  json.RawMessage(`"https://api.individual.githubcopilot.com"`),
			"authorizedAt": json.RawMessage(`1700000000000`),
		},
	}
}

func setCopilotTestCredential(t *testing.T, store *InMemoryCredentialStore, credential *Credential) {
	t.Helper()
	_, err := store.Modify(t.Context(), gitHubCopilotProviderID, func(*Credential) (*Credential, error) {
		return credential, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGitHubCopilotModelDiscoveryGatesBundledCatalog(t *testing.T) {
	baseline := ListModels(gitHubCopilotProviderID)
	if len(baseline) < 3 {
		t.Fatalf("fixture requires at least 3 bundled models, got %d", len(baseline))
	}
	keep, drop := baseline[0].ID, baseline[1].ID
	blocked := baseline[2].ID
	stubCopilotModelsAPI(t, func(request *http.Request) string {
		if request.Method != http.MethodGet || !strings.HasSuffix(request.URL.Path, "/models") {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer fixture-github-token" {
			t.Errorf("request auth header = %q", request.Header.Get("Authorization"))
		}
		// The account picker offers `keep` plus one id the bundled catalog
		// lacks; `drop` is policy-disabled and `blocked` can't tool-call.
		return copilotCatalogBody(
			copilotCatalogRow{keep, true, "enabled", true},
			copilotCatalogRow{"wire-only-model", true, "enabled", true},
			copilotCatalogRow{blocked, true, "enabled", false},
			copilotCatalogRow{drop, true, "disabled", true},
		)
	})

	credentials := NewInMemoryCredentialStore()
	modelsStore := NewInMemoryModelsStore()
	models := CreateModels(CreateModelsOptions{Credentials: credentials, ModelsStore: modelsStore})
	defer func() { models.Close() }()
	provider := copilotDiscoveryTestProvider(t, "https://api.individual.githubcopilot.com")
	models.SetProvider(provider)
	credential := copilotOMPCredential("fixture-github-token")
	setCopilotTestCredential(t, credentials, credential)

	ctx := t.Context()
	result := models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(true), Providers: []string{provider.ID}})
	if len(result.Errors) != 0 {
		t.Fatalf("discovery errors = %v", result.Errors)
	}

	listed, err := provider.GetModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != keep {
		t.Fatalf("roster models = %+v, want only %q", listed, keep)
	}
	available, err := models.GetAvailable(ctx, provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 1 || available[0].ID != keep {
		t.Fatalf("available models = %+v, want only %q", available, keep)
	}
	if known, ok := provider.ModelCatalog.ModelAvailability(provider.BaseURL, credential, drop); !known || ok {
		t.Fatalf("policy-disabled model availability = %v %v", known, ok)
	}
	// The picker-visible wire id without a bundled record is dropped, as
	// upstream's filter keeps only bundled models.
	if _, ok := provider.ModelCatalog.ModelAvailability(provider.BaseURL, credential, "wire-only-model"); ok {
		t.Fatal("unbundled wire id entered the roster")
	}
	if _, ok := provider.ModelCatalog.ModelAvailability(provider.BaseURL, credential, blocked); ok {
		t.Fatal("tool-call-disabled model entered the roster")
	}

	scope := provider.ModelCatalog.Scope(provider.BaseURL, credential)
	if scope.StoreKey == "" {
		t.Fatal("stable credential produced an unpersisted scope")
	}
	entry, err := modelsStore.Read(ctx, scope.StoreKey)
	if err != nil || entry == nil || len(entry.Models) != 1 || entry.Models[0].ModelID() != keep {
		t.Fatalf("persisted roster = %+v, %v", entry, err)
	}
	if strings.Contains(scope.ID, "fixture-github-token") || strings.Contains(scope.StoreKey, "fixture-github-token") {
		t.Fatalf("scope carries credential material: %+v", scope)
	}
}

func TestGitHubCopilotModelDiscoverySeparatesAccounts(t *testing.T) {
	baseline := ListModels(gitHubCopilotProviderID)
	if len(baseline) < 2 {
		t.Fatalf("fixture requires at least 2 bundled models, got %d", len(baseline))
	}
	stubCopilotModelsAPI(t, func(request *http.Request) string {
		if request.Header.Get("Authorization") == "Bearer fixture-token-a" {
			return copilotCatalogBody(copilotCatalogRow{baseline[0].ID, true, "enabled", true})
		}
		return copilotCatalogBody(copilotCatalogRow{baseline[1].ID, true, "enabled", true})
	})

	provider := copilotDiscoveryTestProvider(t, "https://api.individual.githubcopilot.com")
	credentialA := copilotOMPCredential("fixture-token-a")
	credentialB := copilotOMPCredential("fixture-token-b")
	if scopeA, scopeB := provider.ModelCatalog.Scope(provider.BaseURL, credentialA), provider.ModelCatalog.Scope(provider.BaseURL, credentialB); scopeA.StoreKey == "" || scopeA.StoreKey == scopeB.StoreKey {
		t.Fatalf("account scopes not separated: %q vs %q", scopeA.StoreKey, scopeB.StoreKey)
	}

	ctx := t.Context()
	if err := provider.RefreshModels(RefreshModelsContext{Credential: credentialA, BaseURL: provider.BaseURL, AllowNetwork: true, Signal: ctx}); err != nil {
		t.Fatal(err)
	}
	if err := provider.RefreshModels(RefreshModelsContext{Credential: credentialB, BaseURL: provider.BaseURL, AllowNetwork: true, Signal: ctx}); err != nil {
		t.Fatal(err)
	}
	listed, err := provider.GetModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("pooled roster = %d models, want the union of both accounts", len(listed))
	}
	for _, model := range provider.FilterModels(listed, credentialB) {
		if model.ID == baseline[0].ID {
			t.Fatal("account B inherited account A's picker model")
		}
	}
	if known, ok := provider.ModelCatalog.ModelAvailability(provider.BaseURL, credentialB, baseline[0].ID); !known || ok {
		t.Fatalf("account A-only model available to account B: %v %v", known, ok)
	}
}

func TestGitHubCopilotModelDiscoveryRestoresStoredRosterOffline(t *testing.T) {
	baseline := ListModels(gitHubCopilotProviderID)
	if len(baseline) == 0 {
		t.Fatal("fixture requires a bundled catalog")
	}
	keep := baseline[0].ID
	requests := stubCopilotModelsAPI(t, func(*http.Request) string {
		return copilotCatalogBody(copilotCatalogRow{keep, true, "enabled", true})
	})

	credentials := NewInMemoryCredentialStore()
	storePath := filepath.Join(t.TempDir(), "models-store.json")
	credential := copilotOMPCredential("fixture-github-token")
	setCopilotTestCredential(t, credentials, credential)
	ctx := t.Context()

	models := CreateModels(CreateModelsOptions{Credentials: credentials, ModelsStore: NewFileModelsStore(storePath)})
	provider := copilotDiscoveryTestProvider(t, "https://api.individual.githubcopilot.com")
	models.SetProvider(provider)
	if result := models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(true), Providers: []string{provider.ID}}); len(result.Errors) != 0 {
		t.Fatalf("online refresh errors = %v", result.Errors)
	}
	models.Close()

	before := *requests
	models = CreateModels(CreateModelsOptions{Credentials: credentials, ModelsStore: NewFileModelsStore(storePath)})
	defer func() { models.Close() }()
	provider = copilotDiscoveryTestProvider(t, "https://api.individual.githubcopilot.com")
	models.SetProvider(provider)
	if result := models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(false), Providers: []string{provider.ID}}); len(result.Errors) != 0 {
		t.Fatalf("offline refresh errors = %v", result.Errors)
	}
	if *requests != before {
		t.Fatal("offline restore hit the network")
	}
	available, err := models.GetAvailable(ctx, provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 1 || available[0].ID != keep {
		t.Fatalf("offline roster = %+v, want only %q", available, keep)
	}
}

func TestGitHubCopilotModelDiscoveryKeepsBaselineWithoutRoster(t *testing.T) {
	baseline := GetBuiltinModels(gitHubCopilotProviderID)
	provider := copilotDiscoveryTestProvider(t, "https://api.individual.githubcopilot.com")
	// An API-key credential never scopes a roster; the bundled baseline stays.
	apiKey := &Credential{Type: CredentialAPIKey, Key: "fixture-key"}
	if filtered := provider.FilterModels(baseline, apiKey); len(filtered) != len(baseline) {
		t.Fatalf("API-key filter dropped models: %d of %d", len(filtered), len(baseline))
	}
	listed, err := provider.GetModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != len(baseline) {
		t.Fatalf("GetModels without roster = %d, want bundled %d", len(listed), len(baseline))
	}
}
