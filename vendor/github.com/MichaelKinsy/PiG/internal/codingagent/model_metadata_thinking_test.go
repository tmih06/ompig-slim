package codingagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelmetadata"
)

// TestCustomThinkingWithoutMetadataCache exercises the real bundled reference
// path, not injected catalog tiers: a first offline launch must know the same
// model's ladder without weakening explicit model/endpoint opt-outs.
func TestCustomThinkingWithoutMetadataCache(t *testing.T) {
	for _, test := range []struct {
		name       string
		definition string
		want       []ai.ModelThinkingLevel
		wantMap    bool
	}{
		{"id only", `{"id":"deepseek-ai/deepseek-v4.1-flash-1017"}`, []ai.ModelThinkingLevel{ai.ThinkingOff, ai.ThinkingLow, ai.ThinkingHigh, ai.ThinkingMax}, true},
		{"reasoning enabled", `{"id":"deepseek-ai/deepseek-v4.1-flash-1017","reasoning":true}`, []ai.ModelThinkingLevel{ai.ThinkingOff, ai.ThinkingLow, ai.ThinkingHigh, ai.ThinkingMax}, true},
		{"endpoint opt out", `{"id":"deepseek-ai/deepseek-v4.1-flash-1017","reasoning":true,"compat":{"supportsReasoningEffort":false}}`, nil, false},
		{"reasoning disabled", `{"id":"deepseek-ai/deepseek-v4.1-flash-1017","reasoning":false}`, []ai.ModelThinkingLevel{ai.ThinkingOff}, false},
		{"explicit ladder", `{"id":"deepseek-ai/deepseek-v4.1-flash-1017","reasoning":true,"thinkingLevelMap":{"minimal":null,"low":"custom-low","medium":null,"high":null,"xhigh":null,"max":null}}`, []ai.ModelThinkingLevel{ai.ThinkingOff, ai.ThinkingLow}, true},
		{"unknown version", `{"id":"deepseek-ai/deepseek-v99.1-flash-1017","reasoning":true}`, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := NewModelRegistryWithModelsPath(writeModelsJSON(t, `{"providers":{"deploy":{
			  "api":"openai-completions","baseUrl":"http://127.0.0.1:1/v1","apiKey":"fixture",
			  "models":[`+test.definition+`]}}}`))
			if generation := registry.ModelMetadata().Generation(); generation != 0 {
				t.Fatalf("fixture unexpectedly has fetched metadata: generation %d", generation)
			}
			models := registry.GetProviderModelData("deploy")
			if len(models) != 1 {
				t.Fatalf("models = %d; want one custom model", len(models))
			}
			model := models[0]
			if got := model.ThinkingLevelMap != nil; got != test.wantMap {
				t.Fatalf("thinking map = %v; want present=%v", model.ThinkingLevelMap, test.wantMap)
			}
			if test.want != nil && !slices.Equal(ai.GetSupportedThinkingLevels(model), test.want) {
				t.Fatalf("thinking levels = %v; want %v", ai.GetSupportedThinkingLevels(model), test.want)
			}
			if test.name == "explicit ladder" && *model.ThinkingLevelMap[ai.ThinkingLow] != "custom-low" {
				t.Fatal("the bundled ladder overwrote an explicit effort mapping")
			}
		})
	}
}

// TestCustomThinkingRefreshAndOfflineRestart proves that the bundled ladder is
// only a seed: newly fetched tiers replace inherited data and survive an offline
// restart. Neither refresh nor inheritance may alter the custom transport.
func TestCustomThinkingRefreshAndOfflineRestart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"deepseek":{"models":{"deepseek-flash":{
		  "name":"DeepSeek V4.1 Flash 1017","reasoning":true,
		  "reasoning_options":[{"type":"effort","values":["high","max"]}]}}}}`))
	}))
	defer server.Close()
	path := writeModelsJSON(t, `{"providers":{"deploy":{
	  "api":"openai-completions","baseUrl":"http://127.0.0.1:1/v1","apiKey":"fixture",
	  "models":[{"id":"deepseek-ai/deepseek-v4.1-flash-1017","reasoning":true}]}}}`)
	registry := NewModelRegistryWithModelsPath(path)
	registry.SetModelMetadata(modelmetadata.New(modelmetadata.Options{
		Store:      modelmetadata.NewFileStore(filepath.Join(filepath.Dir(path), modelmetadata.FileName)),
		URL:        server.URL,
		Client:     server.Client(),
		References: builtinMetadataReferences,
	}))
	assertLevels := func(registry *ModelRegistry, want []ai.ModelThinkingLevel) {
		t.Helper()
		models := registry.GetProviderModelData("deploy")
		if len(models) != 1 {
			t.Fatalf("models = %d; want one custom model", len(models))
		}
		model := models[0]
		if got := ai.GetSupportedThinkingLevels(model); !slices.Equal(got, want) {
			t.Fatalf("thinking levels = %v; want %v", got, want)
		}
		if model.ProviderMeta.ProviderID != "deploy" || model.ProviderMeta.BaseURL != "http://127.0.0.1:1/v1" || model.ProviderMeta.API != ai.APIOpenAICompletions {
			t.Fatal("effort inheritance altered the custom transport")
		}
	}
	assertLevels(registry, []ai.ModelThinkingLevel{ai.ThinkingOff, ai.ThinkingLow, ai.ThinkingHigh, ai.ThinkingMax})
	if _, err := registry.RefreshModelMetadata(context.Background(), true, true); err != nil {
		t.Fatal(err)
	}
	want := []ai.ModelThinkingLevel{ai.ThinkingOff, ai.ThinkingHigh, ai.ThinkingMax}
	assertLevels(registry, want)
	server.Close()
	assertLevels(NewModelRegistryWithModelsPath(path), want)
}
