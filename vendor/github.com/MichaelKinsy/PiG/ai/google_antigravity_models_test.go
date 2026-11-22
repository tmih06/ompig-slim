package ai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func googleAntigravityDiscoveryTestProvider(t *testing.T, baseURL string) *ModelsProvider {
	t.Helper()
	auth, err := BuiltinProviderAuth(GoogleAntigravityProviderID)
	if err != nil {
		t.Fatal(err)
	}
	provider := &ModelsProvider{
		ID:      GoogleAntigravityProviderID,
		BaseURL: baseURL,
		Auth:    auth,
		GetModels: func() ([]*Model, error) {
			return GetBuiltinModels(GoogleAntigravityProviderID), nil
		},
		GetAllModels: func() ([]AnyModel, error) {
			return AnyModels(GetBuiltinModels(GoogleAntigravityProviderID)), nil
		},
	}
	return WithGoogleAntigravityModelDiscovery(provider)
}

func TestGoogleAntigravityModelDiscoveryScopesEnrichesAndCollapses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != googleCloudCodeAvailableModelsPath {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") == "Bearer token-a" {
			_, _ = io.WriteString(w, `{"models":{
				"claude-opus-4-6":{"supportsImages":true,"supportsThinking":true,"maxTokens":200000,"maxOutputTokens":64000},
				"gemini-3.8-flash-low":{"displayName":"Gemini 3.8 Flash Low","supportsImages":true,"supportsThinking":true,"maxTokens":1000000,"maxOutputTokens":65536},
				"gemini-3.8-flash-medium":{"displayName":"Gemini 3.8 Flash Medium","supportsImages":true,"supportsThinking":true,"maxTokens":1000000,"maxOutputTokens":65536},
				"gemini-3.8-flash-high":{"displayName":"Gemini 3.8 Flash High","supportsImages":true,"supportsThinking":true,"maxTokens":1000000,"maxOutputTokens":65536},
				"hidden-internal":{"isInternal":true},
				"turned-off":{"disabled":true},
				"gemini-2.5-pro":{"displayName":"Denylisted"}
			}}`)
			return
		}
		_, _ = io.WriteString(w, `{"models":{"gemini-3.1-pro":{"supportsImages":true,"supportsThinking":true}}}`)
	}))
	defer server.Close()

	credentials := NewInMemoryCredentialStore()
	storePath := filepath.Join(t.TempDir(), "models-store.json")
	models := CreateModels(CreateModelsOptions{Credentials: credentials, ModelsStore: NewFileModelsStore(storePath)})
	provider := googleAntigravityDiscoveryTestProvider(t, server.URL)
	models.SetProvider(provider)

	credA := &Credential{Type: CredentialOAuth, Access: "token-a", AccountID: "acct-a", ProjectID: "proj-a"}
	credB := &Credential{Type: CredentialOAuth, Access: "token-b", AccountID: "acct-b", ProjectID: "proj-b"}
	if _, err := credentials.Modify(t.Context(), GoogleAntigravityProviderID, func(*Credential) (*Credential, error) { return credA, nil }); err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()
	if result := models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(true), Providers: []string{provider.ID}}); len(result.Errors) != 0 {
		t.Fatalf("online refresh errors = %v", result.Errors)
	}
	available, err := models.GetAvailable(ctx, provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*Model{}
	for _, model := range available {
		byID[model.ID] = model
	}
	if len(byID) != 2 {
		t.Fatalf("expected 2 collapsed live models, got %d: %+v", len(byID), available)
	}
	// Known bundled row preserves its OMP catalog pricing and display name.
	opus := byID["claude-opus-4-6"]
	if opus == nil || opus.Capabilities.InputCostPer1M != 5 || opus.Capabilities.OutputCostPer1M != 25 {
		t.Fatalf("claude-opus-4-6 lost bundled pricing: %+v", opus)
	}
	// Collapsed effort-tier family inherits limits from its default tier SKU.
	flash := byID["gemini-3.8-flash"]
	if flash == nil || flash.Capabilities.ContextWindow != 1000000 || flash.Capabilities.MaxOutputTokens != 65536 {
		t.Fatalf("collapsed gemini-3.8-flash = %+v", flash)
	}
	models.Close()

	// Offline restart restores account A's roster without hitting the network.
	models = CreateModels(CreateModelsOptions{Credentials: credentials, ModelsStore: NewFileModelsStore(storePath)})
	defer models.Close()
	provider = googleAntigravityDiscoveryTestProvider(t, server.URL)
	models.SetProvider(provider)
	if result := models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(false), Providers: []string{provider.ID}}); len(result.Errors) != 0 {
		t.Fatalf("offline refresh errors = %v", result.Errors)
	}
	restored, err := models.GetAvailable(ctx, provider.ID)
	if err != nil || len(restored) != 2 {
		t.Fatalf("offline restored models = %d, %v", len(restored), err)
	}

	// Account B refreshes into its own scope and does not see Account A's roster.
	if err := provider.RefreshModels(RefreshModelsContext{Credential: credB, BaseURL: provider.BaseURL, AllowNetwork: true, Signal: ctx}); err != nil {
		t.Fatal(err)
	}
	filteredB := provider.FilterModels(nil, credB)
	if len(filteredB) != 1 || filteredB[0].ID != "gemini-3.1-pro" {
		t.Fatalf("account B roster = %+v, want only gemini-3.1-pro", filteredB)
	}
	// Broker-redacted credentials without an account discriminator stay ephemeral and separate.
	redactedA := provider.ModelCatalog.Scope(provider.BaseURL, &Credential{Type: CredentialOAuth, Access: "tok-a", Refresh: RemoteRefreshSentinel})
	redactedB := provider.ModelCatalog.Scope(provider.BaseURL, &Credential{Type: CredentialOAuth, Access: "tok-b", Refresh: RemoteRefreshSentinel})
	if redactedA.StoreKey != "" || redactedB.StoreKey != "" || redactedA.ID == "" || redactedA.ID == redactedB.ID {
		t.Fatalf("redacted scopes invalid: %+v vs %+v", redactedA, redactedB)
	}
	if strings.Contains(redactedA.ID, "tok-a") {
		t.Fatalf("scope leaked token: %+v", redactedA)
	}
}
