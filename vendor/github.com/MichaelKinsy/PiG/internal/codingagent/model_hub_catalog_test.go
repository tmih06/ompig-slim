package codingagent

import (
	"slices"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
)

// TestHubCatalogSnapshotMatchesRuntime protects the one-pass projection against
// dropping authenticated models, leaking disabled rows, or hiding locked catalogs.
func TestHubCatalogSnapshotMatchesRuntime(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "OPENROUTER_API_KEY"} {
		t.Setenv(key, "offline-fixture")
	}
	registry := NewModelRegistry(t.TempDir())
	defer registry.CloseModelTasks()
	disabled := []string{"anthropic"}
	available, all := registry.hubCatalogItems(disabled)
	actual := map[string]bool{}
	for _, item := range available {
		if slices.Contains(disabled, item.Provider) {
			t.Fatalf("disabled model offered: %s", item.FQ())
		}
		actual[item.FQ()] = true
	}
	want := map[string]bool{}
	for _, model := range registry.GetAvailableModelData() {
		if !slices.Contains(disabled, model.ProviderID()) {
			want[model.ProviderID()+"/"+model.ID] = true
		}
	}
	for _, item := range all {
		if slices.Contains(disabled, item.Provider) {
			t.Fatalf("disabled model in all: %s", item.FQ())
		}
		if tui.ModelItemKind(item) != "chat" && (ai.OMPKeylessCatalog(item.Provider) || registry.HasConfiguredAuth(item.Provider)) {
			want[item.FQ()] = true
		}
	}
	for id := range want {
		if !actual[id] {
			t.Errorf("missing available model %s", id)
		}
	}
	for id := range actual {
		if !want[id] {
			t.Errorf("unexpected available model %s", id)
		}
	}
	if len(all) <= len(available) {
		t.Fatalf("locked catalogs lost: all=%d available=%d", len(all), len(available))
	}
}
