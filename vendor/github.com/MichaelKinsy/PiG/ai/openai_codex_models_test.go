package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/internal/coding/pigidentity"
)

func TestCodexModelRosterMapsOnlyAdvertisedCapabilities(t *testing.T) {
	body := `{"models":[` +
		`{"slug":"future-discovery","display_name":"Future Discovery","description":"fixture","visibility":"list","supported_in_api":false,"context_window":256000,"supported_reasoning_levels":[{"effort":"low"},{"effort":"high"}],"api":"openai-completions","base_url":"https://attacker.invalid","headers":{"Authorization":"leak"},"max_output_tokens":999999},` +
		`{"slug":"hidden","display_name":"Hidden","visibility":"hide","supported_in_api":true,"input_modalities":["text"]},` +
		`{"slug":"image-only","display_name":"Image only","visibility":"list","supported_in_api":true,"input_modalities":["image"]},` +
		`{"slug":"explicit-empty","display_name":"Empty input","visibility":"list","supported_in_api":true,"input_modalities":[]}]}`
	models, err := parseCodexModelRoster("openai-codex", "http://127.0.0.1:1234", []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("parsed %d models, want one: %+v", len(models), models)
	}
	model := models[0]
	if model.ID != "future-discovery" || model.DisplayName != "Future Discovery" || model.ProviderID() != "openai-codex" {
		t.Fatalf("identity = %+v", model)
	}
	if model.ProviderMeta.API != APIOpenAICodexResponses || model.ProviderMeta.BaseURL != "http://127.0.0.1:1234" || model.ProviderMeta.Headers != nil {
		t.Fatalf("server changed trusted transport: %+v", model.ProviderMeta)
	}
	if model.Capabilities.ContextWindow != 256000 || model.Capabilities.MaxOutputTokens != 0 || model.Capabilities.SupportsImages != true {
		t.Fatalf("capabilities = %+v", model.Capabilities)
	}
	if len(model.Input) != 2 || model.Input[0] != ModelInputText || model.Input[1] != ModelInputImage {
		t.Fatalf("input modalities = %v", model.Input)
	}
	if !model.ProviderMeta.Reasoning || model.Capabilities.MaxThinking != ThinkingLevelHigh {
		t.Fatalf("reasoning capabilities = %+v", model)
	}
	if mapped := model.ThinkingLevelMap[ThinkingLow]; mapped == nil || *mapped != "low" {
		t.Fatalf("low effort mapping = %v", mapped)
	}
	if mapped := model.ThinkingLevelMap[ThinkingHigh]; mapped == nil || *mapped != "high" {
		t.Fatalf("high effort mapping = %v", mapped)
	}
	if model.ThinkingLevelMap[ThinkingMedium] != nil {
		t.Fatal("unadvertised reasoning effort was enabled")
	}
}

func TestCodexModelRosterRejectsMalformedAndAmbiguousMembership(t *testing.T) {
	for name, body := range map[string]string{
		"missing models": `{}`,
		"malformed":      `{"models":[`,
		"blank slug":     `{"models":[{"slug":"  ","visibility":"list","supported_in_api":true,"input_modalities":["text"]}]}`,
		"control slug":   `{"models":[{"slug":"future\nmodel","visibility":"list","supported_in_api":true,"input_modalities":["text"]}]}`,
		"duplicate":      `{"models":[{"slug":"same","visibility":"list","supported_in_api":true,"input_modalities":["text"]},{"slug":"same","visibility":"hide","supported_in_api":false,"input_modalities":["text"]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCodexModelRoster("openai-codex", "https://chatgpt.com/backend-api", []byte(body)); err == nil {
				t.Fatal("invalid roster was accepted")
			}
		})
	}
}

func TestCodexModelsEndpointUsesProviderBaseAndVersion(t *testing.T) {
	for _, test := range []struct{ base, path string }{
		{"https://chatgpt.com/backend-api", "/backend-api/codex/models"},
		{"http://127.0.0.1:1/codex", "/codex/models"},
		{"http://127.0.0.1:1/codex/models", "/codex/models"},
	} {
		endpoint, err := codexModelsEndpoint(test.base)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Path != test.path || parsed.Query().Get("client_version") != codexModelsClientVersion {
			t.Fatalf("endpoint %q parsed=%+v err=%v", endpoint, parsed, err)
		}
	}
	for _, invalid := range []string{"file:///etc/passwd", "https://user:pass@example.invalid", "not a url"} {
		if _, err := codexModelsEndpoint(invalid); err == nil {
			t.Errorf("accepted invalid endpoint %q", invalid)
		}
	}
}

func TestCodexModelDiscoveryRefreshScopesSnapshotsAndKeepsLastGood(t *testing.T) {
	var mu sync.Mutex
	mode := "ok"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/codex/models" || request.URL.Query().Get("client_version") == "" {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.String())
		}
		if request.Header.Get("originator") != pigidentity.CodexOriginator {
			t.Errorf("originator = %q", request.Header.Get("originator"))
		}
		if request.Header.Get("Authorization") != "Bearer fixture-token" || request.Header.Get("ChatGPT-Account-Id") == "" {
			t.Errorf("request auth headers = %+v", request.Header)
		}
		if mode == "failure" {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `fixture-token`)
			return
		}
		if mode == "empty" {
			_, _ = io.WriteString(w, `{"models":[]}`)
			return
		}
		name := "future-account-a"
		if request.Header.Get("ChatGPT-Account-Id") == "account-b" {
			name = "future-account-b"
		}
		w.Header().Set("ETag", `"fixture-roster"`)
		_, _ = fmt.Fprintf(w, `{"models":[{"slug":%q,"display_name":%q,"visibility":"list","supported_in_api":true,"input_modalities":["text"],"context_window":32000}]}`, name, name)
	}))
	defer server.Close()

	credentials := NewInMemoryCredentialStore()
	storePath := filepath.Join(t.TempDir(), "models-store.json")
	modelsStore := NewFileModelsStore(storePath)
	models := CreateModels(CreateModelsOptions{Credentials: credentials, ModelsStore: modelsStore})
	defer func() { models.Close() }()
	provider := codexDiscoveryTestProvider(t, "openai-codex", server.URL)
	models.SetProvider(provider)
	ctx := t.Context()
	setCodexTestCredential(t, credentials, provider.ID, "account-a")

	result := models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(true), Providers: []string{provider.ID}})
	if len(result.Errors) != 0 {
		t.Fatalf("initial refresh errors = %v", result.Errors)
	}
	available, err := models.GetAvailable(ctx, provider.ID)
	if err != nil || !containsModel(available, "future-account-a") {
		t.Fatalf("account A models include new roster: %v %v", containsModel(available, "future-account-a"), err)
	}
	for _, baseline := range GetBuiltinModels(provider.ID) {
		if containsModel(available, baseline.ID) {
			t.Fatalf("authoritative account A roster retained unadvertised baseline %q", baseline.ID)
		}
	}
	allAvailable, err := models.GetAllAvailable(ctx, provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range allAvailable {
		if model.ModelID() != "future-account-a" {
			t.Fatalf("typed availability retained unadvertised model %q", model.ModelID())
		}
	}
	keyA := codexTestStoreKey(provider, &Credential{Type: CredentialOAuth, AccountID: "account-a"})
	storedA, err := modelsStore.Read(ctx, keyA)
	if err != nil || storedA == nil || len(storedA.Models) != 1 || storedA.ETag != `"fixture-roster"` {
		t.Fatalf("stored account A roster = %+v, %v", storedA, err)
	}
	encoded, err := json.Marshal(storedA)
	if err != nil || strings.Contains(string(encoded), "fixture-token") {
		t.Fatalf("stored roster contains credentials: %s, %v", encoded, err)
	}

	setCodexTestCredential(t, credentials, provider.ID, "account-b")
	result = models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(false), Providers: []string{provider.ID}})
	if len(result.Errors) != 0 {
		t.Fatalf("offline account B refresh errors = %v", result.Errors)
	}
	available, err = models.GetAvailable(ctx, provider.ID)
	if err != nil || containsModel(available, "future-account-a") {
		t.Fatalf("account B inherited account A roster: %v %v", containsModel(available, "future-account-a"), err)
	}
	storedB, err := modelsStore.Read(ctx, codexTestStoreKey(provider, &Credential{Type: CredentialOAuth, AccountID: "account-b"}))
	if err != nil || storedB != nil {
		t.Fatalf("account B read another account snapshot: %+v, %v", storedB, err)
	}

	result = models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(true), Providers: []string{provider.ID}})
	if len(result.Errors) != 0 {
		t.Fatalf("account B discovery errors = %v", result.Errors)
	}
	available, err = models.GetAvailable(ctx, provider.ID)
	if err != nil || !containsModel(available, "future-account-b") || containsModel(available, "future-account-a") {
		t.Fatalf("account B roster = A:%v B:%v err:%v", containsModel(available, "future-account-a"), containsModel(available, "future-account-b"), err)
	}

	mu.Lock()
	beforeRestart := requests
	mu.Unlock()
	models.Close()
	models = CreateModels(CreateModelsOptions{Credentials: credentials, ModelsStore: NewFileModelsStore(storePath)})
	provider = codexDiscoveryTestProvider(t, "openai-codex", server.URL)
	models.SetProvider(provider)
	result = models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(false), Providers: []string{provider.ID}})
	if len(result.Errors) != 0 {
		t.Fatalf("offline restart refresh errors = %v", result.Errors)
	}
	available, err = models.GetAvailable(ctx, provider.ID)
	mu.Lock()
	restoredWithoutNetwork := requests == beforeRestart
	mu.Unlock()
	if err != nil || !restoredWithoutNetwork || !containsModel(available, "future-account-b") || containsModel(available, "future-account-a") {
		t.Fatalf("offline restart roster = A:%v B:%v requests unchanged:%v err:%v", containsModel(available, "future-account-a"), containsModel(available, "future-account-b"), restoredWithoutNetwork, err)
	}

	mu.Lock()
	mode = "failure"
	mu.Unlock()
	result = models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(true), Providers: []string{provider.ID}})
	if result.Errors[provider.ID] == nil {
		t.Fatalf("failed discovery was not reported: %v", result.Errors)
	}
	if strings.Contains(result.Errors[provider.ID].Error(), "fixture-token") {
		t.Fatalf("credential leaked in discovery error: %v", result.Errors[provider.ID])
	}
	available, err = models.GetAvailable(ctx, provider.ID)
	if err != nil || !containsModel(available, "future-account-b") {
		t.Fatalf("failed discovery erased last good account B roster: %v", err)
	}

	mu.Lock()
	mode = "empty"
	mu.Unlock()
	result = models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(true), Providers: []string{provider.ID}})
	if len(result.Errors) != 0 {
		t.Fatalf("empty successful discovery errors = %v", result.Errors)
	}
	available, err = models.GetAvailable(ctx, provider.ID)
	if err != nil || containsModel(available, "future-account-b") {
		t.Fatalf("successful empty roster retained stale membership: %v", err)
	}
	for _, baseline := range GetBuiltinModels(provider.ID) {
		if containsModel(available, baseline.ID) {
			t.Fatalf("successful empty roster retained baseline %q", baseline.ID)
		}
	}
	empty, err := modelsStore.Read(ctx, codexTestStoreKey(provider, &Credential{Type: CredentialOAuth, AccountID: "account-b"}))
	if err != nil || empty == nil || len(empty.Models) != 0 {
		t.Fatalf("empty roster was not persisted authoritatively: %+v, %v", empty, err)
	}

	mu.Lock()
	beforeOffline := requests
	mu.Unlock()
	result = models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(false), Providers: []string{provider.ID}})
	mu.Lock()
	afterOffline := requests
	mu.Unlock()
	if afterOffline != beforeOffline || len(result.Errors) != 0 {
		t.Fatalf("offline refresh made a request or failed: requests=%d before=%d errors=%v", afterOffline, beforeOffline, result.Errors)
	}
}

func TestCodexModelDiscoverySanitizesHTTPStatusReason(t *testing.T) {
	oldTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = codexRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			Status:     "503 fixture-token",
			StatusCode: http.StatusServiceUnavailable,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("fixture-token")),
			Request:    request,
		}, nil
	})
	defer func() { http.DefaultClient.Transport = oldTransport }()

	_, _, err := fetchCodexModelRoster(t.Context(), "openai-codex", "https://chatgpt.com", &Credential{Type: CredentialOAuth, Access: "fixture-token"})
	if err == nil || strings.Contains(err.Error(), "fixture-token") || err.Error() != "Codex model discovery returned HTTP 503" {
		t.Fatalf("HTTP error exposed response text: %v", err)
	}
}

func TestCodexModelCacheSeparatesEndpointAndIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(t *testing.T, provider *ModelsProvider, credentials *InMemoryCredentialStore, serverURL string) *ModelsProvider
	}{
		{
			name: "endpoint",
			change: func(_ *testing.T, _ *ModelsProvider, _ *InMemoryCredentialStore, serverURL string) *ModelsProvider {
				return codexDiscoveryTestProvider(t, "openai-codex", serverURL)
			},
		},
		{
			name: "account identity",
			change: func(t *testing.T, provider *ModelsProvider, credentials *InMemoryCredentialStore, _ string) *ModelsProvider {
				_, err := credentials.Modify(t.Context(), provider.ID, func(current *Credential) (*Credential, error) {
					current.Extra = map[string]json.RawMessage{"email": json.RawMessage(`"changed@example.invalid"`)}
					return current, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				return provider
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{"models":[{"slug":"endpoint-a-only","display_name":"Endpoint A","visibility":"list","supported_in_api":true,"input_modalities":["text"]}]}`)
			}))
			defer serverA.Close()
			serverB := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			defer serverB.Close()

			credentials := NewInMemoryCredentialStore()
			store := NewInMemoryModelsStore()
			models := CreateModels(CreateModelsOptions{Credentials: credentials, ModelsStore: store})
			defer models.Close()
			provider := codexDiscoveryTestProvider(t, "openai-codex", serverA.URL)
			models.SetProvider(provider)
			setCodexTestCredential(t, credentials, provider.ID, "scope-account")
			if result := models.Refresh(t.Context(), ModelsRefreshOptions{AllowNetwork: new(true), Providers: []string{provider.ID}}); len(result.Errors) != 0 {
				t.Fatalf("initial refresh errors = %v", result.Errors)
			}

			provider = test.change(t, provider, credentials, serverB.URL)
			models.SetProvider(provider)
			result := models.Refresh(t.Context(), ModelsRefreshOptions{AllowNetwork: new(false), Providers: []string{provider.ID}})
			if len(result.Errors) != 0 {
				t.Fatalf("offline refresh errors = %v", result.Errors)
			}
			available, err := models.GetAvailable(t.Context(), provider.ID)
			if err != nil || containsModel(available, "endpoint-a-only") {
				t.Fatalf("stale roster crossed %s boundary: found=%v err=%v", test.name, containsModel(available, "endpoint-a-only"), err)
			}
		})
	}
}

func TestCodexModelStoreKeysSeparateProvidersAndAccounts(t *testing.T) {
	baseURL := "https://chatgpt.com/backend-api"
	accountA := &Credential{Type: CredentialOAuth, AccountID: "account-a"}
	accountB := &Credential{Type: CredentialOAuth, AccountID: "account-b"}
	openAI := codexDiscoveryTestProvider(t, "openai-codex", baseURL)
	codexJSON := codexDiscoveryTestProvider(t, codexJSONProviderID, baseURL)
	if openAI.ModelCatalog.Scope(baseURL, accountA).StoreKey == openAI.ModelCatalog.Scope(baseURL, accountB).StoreKey {
		t.Fatal("different accounts share a model store key")
	}
	if openAI.ModelCatalog.Scope(baseURL, accountA).StoreKey == codexJSON.ModelCatalog.Scope(baseURL, accountA).StoreKey {
		t.Fatal("Codex providers share a model store key")
	}
	scope := openAI.ModelCatalog.Scope(baseURL, &Credential{Type: CredentialOAuth, Access: "fixture-token"})
	if scope.StoreKey != "" || strings.Contains(scope.ID, "fixture-token") {
		t.Fatalf("token-derived scope was persisted or exposed: %+v", scope)
	}
}

func TestCodexDiscoveryRestoresBaselineMetadataAcrossAccounts(t *testing.T) {
	baseline := GetBuiltinModels("openai-codex")
	if len(baseline) == 0 {
		t.Fatal("Codex fixture requires a built-in baseline")
	}
	baselineID := baseline[0].ID
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"models":[{"slug":%q,"display_name":"Account A metadata","visibility":"list","supported_in_api":true,"input_modalities":["text"]},{"slug":"account-a-only","display_name":"Account A only","visibility":"list","supported_in_api":true,"input_modalities":["text"]}]}`, baselineID)
	}))
	defer server.Close()
	provider := codexDiscoveryTestProvider(t, "openai-codex", server.URL)
	credentialA := &Credential{Type: CredentialOAuth, Access: "account-a-token", AccountID: "account-a"}
	if err := provider.RefreshModels(RefreshModelsContext{Credential: credentialA, BaseURL: server.URL, AllowNetwork: true, Signal: t.Context()}); err != nil {
		t.Fatal(err)
	}
	listed, err := provider.GetModels()
	if err != nil || !containsModel(listed, "account-a-only") {
		t.Fatalf("account A discovery = %v, %v", containsModel(listed, "account-a-only"), err)
	}
	credentialB := &Credential{Type: CredentialOAuth, Access: "account-b-token", AccountID: "account-b"}
	filtered := provider.FilterModels(listed, credentialB)
	if containsModel(filtered, "account-a-only") {
		t.Fatal("account B saw account A-only model")
	}
	foundBaseline := false
	for _, model := range filtered {
		if model.ID == baselineID {
			foundBaseline = true
			if model.DisplayName == "Account A metadata" {
				t.Fatal("account B saw account A-specific metadata for a shared ID")
			}
		}
	}
	if !foundBaseline {
		t.Fatal("account B lost the unpersonalized baseline for a shared ID")
	}
}

func TestCodexProvidersPersistIndependentRosters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") == "" || request.Header.Get("ChatGPT-Account-Id") != "same-account" {
			t.Errorf("request headers = %+v", request.Header)
		}
		_, _ = io.WriteString(w, `{"models":[{"slug":"shared-future-model","display_name":"Shared Future Model","visibility":"list","supported_in_api":true,"input_modalities":["text"]}]}`)
	}))
	defer server.Close()
	credentials := NewInMemoryCredentialStore()
	store := NewInMemoryModelsStore()
	models := CreateModels(CreateModelsOptions{Credentials: credentials, ModelsStore: store})
	defer models.Close()
	for _, providerID := range []string{"openai-codex", codexJSONProviderID} {
		provider := codexDiscoveryTestProvider(t, providerID, server.URL)
		models.SetProvider(provider)
		_, err := credentials.Modify(t.Context(), providerID, func(*Credential) (*Credential, error) {
			return &Credential{Type: CredentialOAuth, Access: "fixture-" + providerID, AccountID: "same-account", Expires: time.Now().Add(time.Hour).UnixMilli()}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		result := models.Refresh(t.Context(), ModelsRefreshOptions{AllowNetwork: new(true), Providers: []string{providerID}})
		if len(result.Errors) != 0 {
			t.Fatalf("%s refresh errors = %v", providerID, result.Errors)
		}
		entry, err := store.Read(t.Context(), codexTestStoreKey(provider, &Credential{Type: CredentialOAuth, AccountID: "same-account"}))
		if err != nil || entry == nil || len(entry.Models) != 1 || entry.Models[0].ProviderID() != providerID {
			t.Fatalf("%s stored roster = %+v, %v", providerID, entry, err)
		}
	}
	for _, providerID := range []string{"openai-codex", codexJSONProviderID} {
		provider := codexDiscoveryTestProvider(t, providerID, server.URL)
		if entry, err := store.Read(t.Context(), codexTestStoreKey(provider, &Credential{Type: CredentialOAuth, AccountID: "same-account"})); err != nil || entry == nil {
			t.Fatalf("%s roster missing after other provider refresh: %+v, %v", providerID, entry, err)
		}
	}
}

func TestCodexModelDiscoveryRetainsUnpersistedRosterOnFailure(t *testing.T) {
	var mu sync.Mutex
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"models":[{"slug":"future-ephemeral","display_name":"Future Ephemeral","visibility":"list","supported_in_api":true,"input_modalities":["text"]}]}`)
	}))
	defer server.Close()
	credentials := NewInMemoryCredentialStore()
	store := NewInMemoryModelsStore()
	models := CreateModels(CreateModelsOptions{Credentials: credentials, ModelsStore: store})
	defer models.Close()
	provider := codexDiscoveryTestProvider(t, "openai-codex", server.URL)
	models.SetProvider(provider)
	_, err := credentials.Modify(t.Context(), provider.ID, func(*Credential) (*Credential, error) {
		return &Credential{Type: CredentialOAuth, Access: "ephemeral-fixture-token", Expires: time.Now().Add(time.Hour).UnixMilli()}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	result := models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(true), Providers: []string{provider.ID}})
	if len(result.Errors) != 0 {
		t.Fatalf("initial refresh errors = %v", result.Errors)
	}
	if entry, err := store.Read(ctx, provider.ID); err != nil || entry != nil {
		t.Fatalf("credential without account ID persisted a roster: %+v, %v", entry, err)
	}
	mu.Lock()
	fail = true
	mu.Unlock()
	result = models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(true), Providers: []string{provider.ID}})
	if result.Errors[provider.ID] == nil {
		t.Fatalf("failed refresh not reported: %v", result.Errors)
	}
	available, err := models.GetAvailable(ctx, provider.ID)
	if err != nil || !containsModel(available, "future-ephemeral") {
		t.Fatalf("unpersisted last-good roster lost: %v", err)
	}
	for _, err := range result.Errors {
		if strings.Contains(err.Error(), "ephemeral-fixture-token") {
			t.Fatalf("credential leaked in error: %v", err)
		}
	}
}

func TestCodexModelDiscoveryCancellationDoesNotPersist(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(entered)
		<-request.Context().Done()
	}))
	defer server.Close()
	credentials := NewInMemoryCredentialStore()
	store := NewInMemoryModelsStore()
	models := CreateModels(CreateModelsOptions{Credentials: credentials, ModelsStore: store})
	defer models.Close()
	provider := codexDiscoveryTestProvider(t, "openai-codex", server.URL)
	models.SetProvider(provider)
	setCodexTestCredential(t, credentials, provider.ID, "cancel-account")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan ModelsRefreshResult, 1)
	go func() {
		done <- models.Refresh(ctx, ModelsRefreshOptions{AllowNetwork: new(true), Providers: []string{provider.ID}})
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("discovery request did not start")
	}
	cancel()
	result := <-done
	if !result.Aborted {
		t.Fatalf("cancelled discovery was not aborted: %+v", result)
	}
	if entry, err := store.Read(t.Context(), codexTestStoreKey(provider, &Credential{Type: CredentialOAuth, AccountID: "cancel-account"})); err != nil || entry != nil {
		t.Fatalf("cancelled roster was persisted: %+v, %v", entry, err)
	}
}

type codexRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f codexRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func codexTestStoreKey(provider *ModelsProvider, credential *Credential) string {
	return provider.ModelCatalog.Scope(provider.BaseURL, credential).StoreKey
}

func codexDiscoveryTestProvider(t *testing.T, providerID, baseURL string) *ModelsProvider {
	t.Helper()
	auth, err := BuiltinProviderAuth(providerID)
	if err != nil {
		t.Fatal(err)
	}
	provider := &ModelsProvider{
		ID:      providerID,
		BaseURL: baseURL,
		Auth:    auth,
		GetModels: func() ([]*Model, error) {
			return GetBuiltinModels(providerID), nil
		},
		GetAllModels: func() ([]AnyModel, error) {
			return AnyModels(GetBuiltinModels(providerID)), nil
		},
	}
	return WithCodexModelDiscovery(provider)
}

func setCodexTestCredential(t *testing.T, store *InMemoryCredentialStore, providerID, accountID string) {
	t.Helper()
	_, err := store.Modify(t.Context(), providerID, func(*Credential) (*Credential, error) {
		return &Credential{Type: CredentialOAuth, Access: "fixture-token", AccountID: accountID, Expires: time.Now().Add(time.Hour).UnixMilli()}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func containsModel(models []*Model, id string) bool {
	for _, model := range models {
		if model.ID == id {
			return true
		}
	}
	return false
}
