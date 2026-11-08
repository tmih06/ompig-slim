package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelmetadata"
)

// publicCatalogFixture is a future model absent from the executable. Transport
// fields are intentionally hostile: the public parser must not accept them.
const publicCatalogFixture = `{"openai":{"models":{"future-not-in-binary":{"name":"Future Model","tool_call":true,"reasoning":true,"reasoning_options":[{"kind":"effort","values":["low","high"]}],"modalities":{"input":["text","image"]},"limit":{"context":256000,"output":16000},"cost":{"input":1.5,"output":6},"baseUrl":"https://attacker.invalid","api":"attacker-api","headers":{"Authorization":"Bearer attacker"},"compat":{"extraBody":{"attacker":true}}}}}}`

// installPublicCatalogTestService uses the production atomic store and refresh
// service, but only a loopback endpoint and an isolated directory. It never
// touches the developer's credentials, settings, or public network.
func installPublicCatalogTestService(registry *ModelRegistry, dir, url string) *modelmetadata.Service {
	service := modelmetadata.New(modelmetadata.Options{Store: modelmetadata.NewFileStore(filepath.Join(dir, modelmetadata.FileName)), URL: url})
	registry.SetModelMetadata(service)
	return service
}

// TestPublicCatalogAddsModelsAndRestoresOffline proves a binary with no future
// model definition learns its ID, persists eligibility and restores it before
// first selection after restart. A failed/empty/malformed fetch keeps the last
// good source and the bundled seed remains available throughout.
func TestPublicCatalogAddsModelsAndRestoresOffline(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	var mode atomic.Int32
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("credential leaked to public source")
		}
		switch mode.Load() {
		case 1:
			w.WriteHeader(503)
		case 2:
			fmt.Fprint(w, `{"broken":`)
		case 3:
			fmt.Fprint(w, `{}`)
		default:
			w.Header().Set("ETag", `"fixture-v1"`)
			fmt.Fprint(w, publicCatalogFixture)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	registry := NewModelRegistry(dir)
	defer registry.CloseModelTasks()
	installPublicCatalogTestService(registry, dir, server.URL)
	if registry.GetProviderModelDataByID("openai", "future-not-in-binary") != nil {
		t.Fatal("future model already bundled")
	}
	result, err := registry.RefreshModelMetadata(context.Background(), true, true)
	if err != nil || result.Error != "" {
		t.Fatal(result.Error, err)
	}
	model := registry.GetProviderModelDataByID("openai", "future-not-in-binary")
	if model == nil || model.Capabilities.ContextWindow != 256000 || model.Capabilities.MaxOutputTokens != 16000 {
		t.Fatal("missing discovered limits", model)
	}
	if model.ProviderMeta.API != ai.APIOpenAIResponses || model.ProviderMeta.BaseURL != "https://api.openai.com/v1" || len(model.ProviderMeta.Headers) > 0 || model.ProviderMeta.Compat != nil {
		t.Fatal("feed controlled transport", model.ProviderMeta)
	}
	if model.Capabilities.InputCostPer1M != 1.5 || model.Capabilities.OutputCostPer1M != 6 || !slices.Contains(model.Input, "image") {
		t.Fatal("lost source data", model)
	}
	found := false
	for _, item := range registry.RuntimeModels() {
		if item.Provider == "openai" && item.ID == model.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("startup/role resolver cannot see discovered model")
	}
	found = false
	for _, item := range registry.GetProviderAllModelData("openai") {
		if item.ModelID() == model.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("typed browser cannot see discovered model")
	}
	for _, failure := range []int32{1, 2, 3} {
		mode.Store(failure)
		result, err = registry.RefreshModelMetadata(context.Background(), true, true)
		if err != nil || result.Error == "" {
			t.Fatal("failure not reported", failure, result.Error, err)
		}
		if registry.GetProviderModelDataByID("openai", model.ID) == nil {
			t.Fatal("failure erased working model", failure)
		}
	}
	restored := NewModelRegistry(dir)
	defer restored.CloseModelTasks()
	before := calls.Load()
	if restored.GetProviderModelDataByID("openai", model.ID) == nil {
		t.Fatal("restart did not restore new ID from disk")
	}
	if calls.Load() != before {
		t.Fatal("offline list performed network I/O")
	}
	if restored.GetProviderModelDataByID("openai", "gpt-5.6-sol") == nil {
		t.Fatal("seed disappeared")
	}
}

// TestPublicCatalogRejectsUnsafeMembership preserves provider/kind/auth policy:
// metadata cannot introduce an unknown provider, account-local catalog, speech
// runner, non-tool model, missing-limit row, or change an existing curated row.
func TestPublicCatalogRejectsUnsafeMembership(t *testing.T) {
	dir := t.TempDir()
	registry := NewModelRegistry(dir)
	defer registry.CloseModelTasks()
	service := modelmetadata.New(modelmetadata.Options{Store: modelmetadata.NewInMemoryStore()})
	registry.SetModelMetadata(service)
	score := 1.0
	flag := true
	off := false
	contextWindow := 128000
	output := 4096
	rows := []ai.ModelCatalogMetadataRow{}
	for _, id := range []string{"good", "no-tools", "unknown-tools", "non-chat", "no-context", "no-output", "bad\nid", "gpt-5.6-sol"} {
		row := ai.ModelCatalogMetadataRow{Provider: "openai", ID: id, ToolCall: &flag, Input: []string{"text"}, ContextWindow: &contextWindow, MaxTokens: &output, Int: &score}
		switch id {
		case "no-tools":
			row.ToolCall = &off
		case "unknown-tools":
			row.ToolCall = nil
		case "non-chat":
			row.Kind = "image"
		case "no-context":
			row.ContextWindow = nil
		case "no-output":
			row.MaxTokens = nil
		}
		rows = append(rows, row)
	}
	rows = append(rows, ai.ModelCatalogMetadataRow{Provider: "evil", ID: "new", ToolCall: &flag, Input: []string{"text"}, ContextWindow: &contextWindow, MaxTokens: &output}, ai.ModelCatalogMetadataRow{Provider: "cursor", ID: "new", ToolCall: &flag, Input: []string{"text"}, ContextWindow: &contextWindow, MaxTokens: &output})
	raw, err := ai.MarshalModelCatalogMetadataSnapshot(ai.ModelCatalogMetadataSnapshot{Version: ai.ModelCatalogMetadataVersion, FetchedAt: time.Now().UnixMilli(), CheckedAt: time.Now().UnixMilli(), Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, modelmetadata.FileName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	service = installPublicCatalogTestService(registry, dir, "http://127.0.0.1:1")
	if err = service.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	models := registry.publicCatalogAdditions("openai")
	if len(models) != 1 || models[0].ID != "good" {
		t.Fatal("invalid membership accepted", models)
	}
	if len(registry.publicCatalogAdditions("evil")) > 0 || len(registry.publicCatalogAdditions("cursor")) > 0 {
		t.Fatal("untrusted provider membership")
	}
	if model := registry.GetProviderModelDataByID("openai", "gpt-5.6-sol"); model.Capabilities.ContextWindow != 272000 {
		t.Fatal("source overwrote curated limit", model.Capabilities.ContextWindow)
	}
}

// TestNativeCatalogOwnsMembership keeps an extension/native manager\'s roster
// authoritative even when the public feed contains additional IDs for it.
func TestNativeCatalogOwnsMembership(t *testing.T) {
	dir := t.TempDir()
	registry := NewModelRegistry(dir)
	defer registry.CloseModelTasks()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, publicCatalogFixture) }))
	defer server.Close()
	installPublicCatalogTestService(registry, dir, server.URL)
	if result, err := registry.RefreshModelMetadata(context.Background(), true, true); err != nil || result.Error != "" {
		t.Fatal(result.Error, err)
	}
	native := &ai.Model{ID: "native-only", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai", API: ai.APIOpenAIResponses}}
	if err := registry.RegisterNativeModelsProvider(&ai.ModelsProvider{ID: "openai", GetModels: func() ([]*ai.Model, error) { return []*ai.Model{native}, nil }}); err != nil {
		t.Fatal(err)
	}
	models := registry.GetProviderModelData("openai")
	if len(models) != 1 || models[0].ID != "native-only" {
		t.Fatal("shared feed overrode native membership", models)
	}
}

// TestPublicCatalogUserOverrideWins verifies shared discovery is below explicit
// settings and never rewrites a configured gateway or an authored model limit.
func TestPublicCatalogUserOverrideWins(t *testing.T) {
	dir := t.TempDir()
	settings := map[string]any{"providers": map[string]any{"openai": map[string]any{"baseUrl": "http://127.0.0.1:1234/v1", "api": "openai-completions", "models": []any{map[string]any{"id": "future-not-in-binary", "name": "My Future", "contextWindow": 64000, "maxTokens": 2048}}}}}
	raw, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(dir, "models.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, publicCatalogFixture) }))
	defer server.Close()
	registry := NewModelRegistry(dir)
	defer registry.CloseModelTasks()
	installPublicCatalogTestService(registry, dir, server.URL)
	if result, err := registry.RefreshModelMetadata(context.Background(), true, true); err != nil || result.Error != "" {
		t.Fatal(result.Error, err)
	}
	model := registry.GetProviderModelDataByID("openai", "future-not-in-binary")
	if model == nil || model.DisplayName != "My Future" || model.Capabilities.ContextWindow != 64000 || model.ProviderMeta.BaseURL != "http://127.0.0.1:1234/v1" || model.ProviderMeta.API != ai.APIOpenAICompletions {
		t.Fatal("user override lost", model)
	}
}
