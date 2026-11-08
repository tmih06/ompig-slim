package codingagent

import (
	"github.com/MichaelKinsy/PiG/ai"
	"testing"
)

// TestBrowserCatalogContainsEveryOMPIdentity covers the actual composed runtime
// list. Native classifier descriptors may back OMP judge rows, but their display
// kind comes from the canonical catalog and must not overwrite a chat namesake.
func TestBrowserCatalogContainsEveryOMPIdentity(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	registry := NewModelRegistry(t.TempDir())
	defer registry.CloseModelTasks()
	for _, provider := range ai.OMPCatalogProviders() {
		expected := map[string]string{}
		chat, _ := ai.OMPChatCatalog(provider)
		for _, model := range chat {
			expected[model.ID] = "chat"
		}
		for _, model := range ai.OMPOtherCatalog(provider) {
			expected[model.ModelID()] = string(model.ModelType())
		}
		actual := map[string]string{}
		for _, model := range registry.GetProviderAllModelData(provider) {
			item := hubCatalogItem(model)
			if _, duplicate := actual[item.ID]; duplicate {
				t.Fatalf("duplicate %s/%s", provider, item.ID)
			}
			actual[item.ID] = item.Kind
		}
		if len(actual) != len(expected) {
			t.Fatalf("%s: got %d want %d", provider, len(actual), len(expected))
		}
		for id, kind := range expected {
			if actual[id] != kind {
				t.Fatalf("%s/%s: got %q want %q", provider, id, actual[id], kind)
			}
		}
	}
}
