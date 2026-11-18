package codingagent

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelmetadata"
)

func TestCodexDiscoveryFlowsThroughRegistryAndRestoresOffline(t *testing.T) {
	var calls atomic.Int32
	var empty atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodGet || request.URL.Path != "/codex/models" || request.URL.Query().Get("client_version") == "" {
			t.Errorf("unexpected discovery request: %s %s", request.Method, request.URL.String())
		}
		if request.Header.Get("Authorization") != "Bearer registry-fixture-token" || request.Header.Get("ChatGPT-Account-Id") != "registry-account" {
			t.Errorf("request auth headers = %+v", request.Header)
		}
		if request.Header.Get("originator") == "" {
			t.Error("missing Codex originator")
		}
		if empty.Load() {
			_, _ = fmt.Fprint(w, `{"models":[]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"models":[{"slug":"future-registry-model","display_name":"Remote Name","visibility":"list","supported_in_api":true,"input_modalities":["text"],"context_window":64000}]}`)
	}))
	defer server.Close()

	modelsPath := writeModelsJSON(t, fmt.Sprintf(`{"providers":{"openai-codex":{"api":"openai-codex-responses","baseUrl":%q,"models":[{"id":"future-registry-model","name":"Configured Name","contextWindow":12345,"maxTokens":4096},{"id":"explicit-only","name":"Explicit Only","contextWindow":16000,"maxTokens":2048}]}}}`, server.URL))
	agentDir := filepath.Dir(modelsPath)
	credentials := newCodexRegistryCredentials(t, "openai-codex")
	registry := NewModelRegistry(agentDir)
	closed := false
	defer func() {
		if !closed {
			registry.CloseModelTasks()
		}
	}()
	registry.SetCredentialStore(credentials)
	registry.SetModelMetadata(modelmetadata.New(modelmetadata.Options{Store: modelmetadata.NewInMemoryStore()}))

	result := registry.RefreshCatalogs(t.Context(), CatalogRefreshOptions{AllowNetwork: true, Providers: []string{"openai-codex"}})
	if len(result.Errors) != 0 {
		t.Fatalf("registry refresh errors = %v", result.Errors)
	}
	available, err := registry.GetAvailableModelDataContext(t.Context(), "openai-codex")
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(available, func(model *ai.Model) bool { return model.ID == "future-registry-model" })
	if index < 0 {
		t.Fatal("registry availability omitted the authenticated live model")
	}
	model := available[index]
	if model.DisplayName != "Configured Name" || model.Capabilities.ContextWindow != 12345 || model.Capabilities.MaxOutputTokens != 4096 {
		t.Fatalf("custom definition did not take precedence: %+v", model)
	}
	catalog := registry.GetProviderModelData("openai-codex")
	if len(catalog) != 2 || !slices.ContainsFunc(catalog, func(model *ai.Model) bool { return model.ID == "explicit-only" }) {
		t.Fatalf("live catalog did not preserve explicit custom models: %+v", catalog)
	}
	for _, catalogModel := range catalog {
		if catalogModel.ID != "future-registry-model" && catalogModel.ID != "explicit-only" {
			t.Fatalf("authoritative roster retained unadvertised baseline model %q", catalogModel.ID)
		}
	}
	if !slices.ContainsFunc(registry.GetProviderAllModelData("openai-codex"), func(model ai.AnyModel) bool { return model.ModelID() == "future-registry-model" }) {
		t.Fatal("typed model catalog omitted the live model")
	}

	registry.CloseModelTasks()
	closed = true
	restarted := NewModelRegistry(agentDir)
	defer restarted.CloseModelTasks()
	restarted.SetCredentialStore(newCodexRegistryCredentials(t, "openai-codex"))
	restarted.SetModelMetadata(modelmetadata.New(modelmetadata.Options{Store: modelmetadata.NewInMemoryStore()}))
	before := calls.Load()
	result = restarted.RefreshCatalogs(t.Context(), CatalogRefreshOptions{AllowNetwork: false, Providers: []string{"openai-codex"}})
	if len(result.Errors) != 0 {
		t.Fatalf("offline restore errors = %v", result.Errors)
	}
	available, err = restarted.GetAvailableModelDataContext(t.Context(), "openai-codex")
	if err != nil || !slices.ContainsFunc(available, func(model *ai.Model) bool { return model.ID == "future-registry-model" }) {
		t.Fatalf("restarted registry omitted persisted model: %v, %v", available, err)
	}
	if after := calls.Load(); after != before {
		t.Fatalf("offline restart made %d network requests", after-before)
	}
	empty.Store(true)
	result = restarted.RefreshCatalogs(t.Context(), CatalogRefreshOptions{AllowNetwork: true, Providers: []string{"openai-codex"}})
	if len(result.Errors) != 0 {
		t.Fatalf("empty roster refresh errors = %v", result.Errors)
	}
	models := restarted.GetProviderModelData("openai-codex")
	if len(models) != 2 || !slices.ContainsFunc(models, func(model *ai.Model) bool {
		return model.ID == "future-registry-model" && model.DisplayName == "Configured Name"
	}) || !slices.ContainsFunc(models, func(model *ai.Model) bool { return model.ID == "explicit-only" }) {
		t.Fatalf("successful empty roster removed explicit custom models: %+v", models)
	}
	available, err = restarted.GetAvailableModelDataContext(t.Context(), "openai-codex")
	if err != nil || len(available) != 2 {
		t.Fatalf("available models after empty roster = %+v, %v", available, err)
	}
}

func TestCodexJSONRegistryKeepsItsNativeDiscoveryOwner(t *testing.T) {
	var nativeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/codex/models":
			nativeCalls.Add(1)
			_, _ = fmt.Fprint(w, `{"models":[{"slug":"future-codex-json","display_name":"Future Codex JSON","visibility":"list","supported_in_api":true,"input_modalities":["text"]}]}`)
		case "/models":
			_, _ = fmt.Fprint(w, `{"models":[]}`)
		default:
			_, _ = fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()

	modelsPath := writeModelsJSON(t, fmt.Sprintf(`{"providers":{"codex-json":{"api":"openai-codex-responses","baseUrl":%q}}}`, server.URL))
	registry := NewModelRegistry(filepath.Dir(modelsPath))
	defer registry.CloseModelTasks()
	registry.SetCredentialStore(newCodexRegistryCredentials(t, "codex-json"))
	registry.SetModelMetadata(modelmetadata.New(modelmetadata.Options{Store: modelmetadata.NewInMemoryStore()}))
	registry.SetCatalogBaseURL(server.URL)
	result := registry.RefreshCatalogs(t.Context(), CatalogRefreshOptions{AllowNetwork: true, Providers: []string{"codex-json"}})
	if len(result.Errors) != 0 {
		t.Fatalf("codex-json refresh errors = %v", result.Errors)
	}
	if nativeCalls.Load() == 0 {
		t.Fatal("codex-json's native /codex/models discovery callback was replaced")
	}
	if !slices.ContainsFunc(registry.GetProviderModelData("codex-json"), func(model *ai.Model) bool { return model.ID == "future-codex-json" }) {
		t.Fatal("registry omitted the native codex-json roster")
	}
}

func TestCodexDiscoveryUnionsAndRoutesAcrossStoredAccounts(t *testing.T) {
	var discoveryCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/codex/models" {
			discoveryCalls.Add(1)
			modelID := "first-only"
			if request.Header.Get("ChatGPT-Account-Id") == "registry-second" {
				modelID = "second-only"
			}
			_, _ = fmt.Fprintf(w, `{"models":[{"slug":%q,"display_name":%q,"visibility":"list","supported_in_api":false,"input_modalities":["text"]}]}`, modelID, modelID)
			return
		}
		_, _ = fmt.Fprint(w, `{"fixture":{"models":{"one":{"name":"One","int":42,"tps":123}}}}`)
	}))
	defer server.Close()

	modelsPath := writeModelsJSON(t, fmt.Sprintf(`{"providers":{"openai-codex":{"api":"openai-codex-responses","baseUrl":%q}}}`, server.URL))
	agentDir := filepath.Dir(modelsPath)
	credentials, err := ai.OpenSQLiteAuthStorage(filepath.Join(agentDir, "auth.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = credentials.Close() }()
	for _, account := range []struct{ key, accountID string }{{"first", "registry-first"}, {"second", "registry-second"}} {
		key := account.key
		_, err := credentials.SaveAccount(t.Context(), "openai-codex", &key, ai.Credential{
			Type: ai.CredentialOAuth, Access: codexRegistryJWT(account.accountID), AccountID: account.accountID,
			Expires: time.Now().Add(time.Hour).UnixMilli(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if selected, err := credentials.SelectAccount(t.Context(), "openai-codex", "first"); err != nil || !selected {
		t.Fatalf("select first account: selected=%v err=%v", selected, err)
	}
	registry := NewModelRegistry(agentDir)
	defer registry.CloseModelTasks()
	registry.SetCredentialStore(credentials)
	registry.SetModelMetadata(modelmetadata.New(modelmetadata.Options{Store: modelmetadata.NewInMemoryStore(), URL: server.URL, Client: server.Client()}))
	result := registry.RefreshCatalogs(t.Context(), CatalogRefreshOptions{AllowNetwork: true, Providers: []string{"openai-codex"}})
	if len(result.Errors) != 0 {
		t.Fatalf("account discovery errors = %v", result.Errors)
	}
	if discoveryCalls.Load() != 2 {
		t.Fatalf("discovery calls = %d, want both accounts", discoveryCalls.Load())
	}
	models := registry.GetProviderModelData("openai-codex")
	if len(models) != 2 || !slices.ContainsFunc(models, func(model *ai.Model) bool { return model.ID == "first-only" }) || !slices.ContainsFunc(models, func(model *ai.Model) bool { return model.ID == "second-only" }) {
		t.Fatalf("account rosters were not unioned: %+v", models)
	}
	second := registry.GetProviderModelDataByID("openai-codex", "second-only")
	if second == nil {
		t.Fatal("second-account-only model is missing")
	}
	var selected []string
	stream := registry.AccountRouter().StreamRoutes(t.Context(), second, ai.AccountRouteRequest{SessionID: "registry-routing"}, func(_ context.Context, selection ai.AccountSelection, _ ai.AccountObserve, _ bool) (*ai.AssistantMessageEventStream, error) {
		selected = append(selected, *selection.AccountKey)
		return codexRegistrySuccessStream(second.ID), nil
	})
	for range stream.Events(t.Context()) {
	}
	if _, err := stream.ResultContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || selected[0] != "second" {
		t.Fatalf("second-only model routed to accounts %v", selected)
	}

	if deleted, err := credentials.DeleteAccount(t.Context(), "openai-codex", "first"); err != nil || !deleted {
		t.Fatalf("delete first account: deleted=%v err=%v", deleted, err)
	}
	result = registry.RefreshCatalogs(t.Context(), CatalogRefreshOptions{AllowNetwork: false, Providers: []string{"openai-codex"}})
	if len(result.Errors) != 0 {
		t.Fatalf("offline account cleanup errors = %v", result.Errors)
	}
	if discoveryCalls.Load() != 2 {
		t.Fatalf("offline account cleanup made discovery requests: %d", discoveryCalls.Load())
	}
	models = registry.GetProviderModelData("openai-codex")
	if len(models) != 1 || models[0].ID != "second-only" {
		t.Fatalf("removed account roster remained in composition: %+v", models)
	}
}

func codexRegistryJWT(accountID string) string {
	payload := fmt.Sprintf(`{"sub":%q,"https://api.openai.com/auth":{"chatgpt_account_id":%q,"chatgpt_plan_type":"plus"},"https://api.openai.com/profile":{"email":%q}}`, accountID, accountID, accountID+"@example.invalid")
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".fixture"
}

func codexRegistrySuccessStream(modelID string) *ai.AssistantMessageEventStream {
	stream := ai.NewAssistantMessageEventStream()
	partial := &ai.AssistantMessage{Provider: "openai-codex", Model: modelID, Content: []ai.AssistantContentBlock{ai.TextContent{Text: "ok"}}}
	_ = stream.Push(ai.StartEvent{Partial: partial})
	final := &ai.AssistantMessage{Provider: "openai-codex", Model: modelID, Content: []ai.AssistantContentBlock{ai.TextContent{Text: "ok"}}, StopReason: ai.StopReasonStop}
	_ = stream.Push(ai.DoneEvent{Reason: ai.StopReasonStop, Message: final})
	return stream
}

func newCodexRegistryCredentials(t *testing.T, providerID string) *ai.InMemoryCredentialStore {
	t.Helper()
	store := ai.NewInMemoryCredentialStore()
	_, err := store.Modify(t.Context(), providerID, func(*ai.Credential) (*ai.Credential, error) {
		return &ai.Credential{Type: ai.CredentialOAuth, Access: "registry-fixture-token", AccountID: "registry-account", Expires: time.Now().Add(time.Hour).UnixMilli()}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}
