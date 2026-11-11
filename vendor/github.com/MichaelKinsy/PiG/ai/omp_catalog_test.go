package ai

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"
)

// TestOMPCatalogMatchesManifest traverses the complete pinned snapshot, not a
// two-model fixture. Every row must retain provider/id/kind and no duplicate can
// silently disappear during compact decoding or typed catalog composition.
func TestOMPCatalogMatchesManifest(t *testing.T) {
	raw, err := os.ReadFile("omp_catalog_data/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version   string
		Providers map[string]int
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "18.14.2" {
		t.Fatal(manifest.Version)
	}
	if len(OMPCatalogProviders()) != 90 || HasOMPCatalog("metric-aliases") {
		t.Fatal("metadata index leaked into provider roster")
	}
	total := 0
	kinds := map[ModelType]int{}
	for provider, want := range manifest.Providers {
		catalog := loadOMPCatalog(provider)
		if catalog == nil {
			t.Fatal("missing provider", provider)
		}
		rows := append(slices.Clone(catalog.chat), catalog.other...)
		if len(rows) != want {
			t.Fatalf("%s: decoded=%d want=%d", provider, len(rows), want)
		}
		for _, row := range rows {
			if row.Provider != provider || row.ID == "" {
				t.Fatal("lost identity", provider, row.ID)
			}
			kind := row.Kind
			if kind == "" {
				kind = ModelTypeChat
			}
			kinds[kind]++
		}
		total += len(rows)
	}
	if total != 5949 || kinds[ModelTypeChat] != 5703 {
		t.Fatal(total, kinds)
	}
	for _, kind := range []ModelType{"tiny", "judge", "image", "search", "tts", "stt", "embedding", "rerank", "video"} {
		if kinds[kind] == 0 {
			t.Fatal("missing kind", kind)
		}
	}
}

// TestOMPChatLookupAndListingShareCatalog prevents the old split where the menu
// offered a new model but startup, bare lookup, or a subagent saw the stale list.
func TestOMPChatLookupAndListingShareCatalog(t *testing.T) {
	models := ListModels("openai")
	if len(models) != 61 {
		t.Fatal("OpenAI roster", len(models))
	}
	seen := map[string]bool{}
	for _, model := range models {
		seen[model.ID] = true
	}
	for _, id := range []string{"gpt-5.6", "gpt-5.1-codex-max", "gpt-5.6-luna-pro", "daybreak-blue-latest"} {
		if !seen[id] {
			t.Fatal("missing", id)
		}
		model, ok := LookupModelExact("openai/" + id)
		if !ok || model.ID != id {
			t.Fatal("exact lookup", id)
		}
	}
	if seen["chat-latest"] {
		t.Fatal("stale alias leaked into catalog")
	}
	if model, ok := LookupModel("openai/chat-latest"); ok && model.Provider == "openai" {
		t.Fatal("fuzzy lookup resurrected a retired owned entry")
	}
	if model, ok := LookupModel("gpt-5.6-luna-pro"); !ok || model.ID != "gpt-5.6-luna-pro" {
		t.Fatal("bare lookup")
	}
	if model, _ := LookupModelExact("openai/gpt-5.6"); model.ContextWindow != 272000 {
		t.Fatal("premium context not capped", model.ContextWindow)
	}
	for _, model := range ListModels("openrouter") {
		if _, _, variant := OpenRouterRoutingVariant(model.Provider, model.ID); variant {
			t.Fatal("synthetic routing variant listed", model.ID)
		}
	}
}

// TestOMPChatIndexesPreserveProviderPrecedence checks every seed row against
// exact lookup and verifies that bare lookup retains the first provider's full
// row, including duplicate IDs. The index must not change catalog ordering or
// expose mutable row ownership when its internal representation changes.
func TestOMPChatIndexesPreserveProviderPrecedence(t *testing.T) {
	first := map[string]GeneratedModel{}
	for _, provider := range OMPCatalogProviders() {
		rows, _ := OMPChatCatalog(provider)
		for _, row := range rows {
			if got, ok := LookupOMPChatModel(provider, row.ID); !ok || !reflect.DeepEqual(got, row) {
				t.Fatalf("exact lookup changed %s/%s", provider, row.ID)
			}
			if _, exists := first[row.ID]; !exists {
				first[row.ID] = row
			}
		}
	}
	for id, want := range first {
		got, ok := LookupOMPChatModelByID(id)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("bare lookup changed %s: got %s, want %s", id, got.Provider, want.Provider)
		}
		got.ID = "caller-owned-copy"
		if again, _ := LookupOMPChatModelByID(id); again.ID != id {
			t.Fatalf("caller mutated the index row for %s", id)
		}
	}
	if _, ok := LookupOMPChatModelByID("not-a-bundled-id"); ok {
		t.Fatal("missing bare ID resolved")
	}
}

// TestOMPMetricsRemainRefreshable distinguishes bundled metadata from an
// explicit user override so public catalog refreshes can update stale scores.
func TestOMPMetricsRemainRefreshable(t *testing.T) {
	generated, ok := LookupModelExact("openai/gpt-6.1-sol")
	if !ok {
		t.Fatal("missing model")
	}
	model := generated.ToModel()
	if model.Int == nil || model.Catalog == nil || len(model.Catalog.Fields) == 0 {
		t.Fatal("lost metadata provenance")
	}
	if keys := OMPProviderEnvKeys("stepfun"); len(keys) != 1 || keys[0] != "STEPFUN_API_KEY" {
		t.Fatal(keys)
	}
	rows, _ := OMPChatCatalog("local")
	if len(rows) != 0 {
		t.Fatal("non-chat runner became a chat model")
	}
}
