package codingagent

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
)

type accountFilterCredentials struct {
	ai.CredentialStore
	credential ai.Credential
}

func (s accountFilterCredentials) GetRaw(string) (ai.Credential, bool, error) {
	return s.credential, true, nil
}

func TestAccountFilterDoesNotReadItsOwnPublishedProjection(t *testing.T) {
	registry := NewModelRegistry(t.TempDir())
	defer registry.CloseModelTasks()
	registry.SetCredentialStore(accountFilterCredentials{credential: ai.Credential{
		Type:              ai.CredentialOAuth,
		AvailableModelIDs: json.RawMessage(`["allowed"]`),
	}})
	registry.SetAvailabilitySnapshot(
		func(string) bool { return true },
		nil,
		func(_, id string) bool { return id == "published" },
		func(string, *ai.AuthCheck, func() []string) {},
	)
	live := registry.accountModelFilter("github-copilot")
	if !live("allowed") || live("other") {
		t.Fatal("live account availability filter must come from credential state, not the projection it is computing")
	}
	published := registry.publishedAccountModelFilter("github-copilot")
	if !published("published") || published("allowed") {
		t.Fatal("interactive presentation must read the published projection without reopening credentials")
	}
}

func TestHubOpenCodeFreeWithoutCredentials(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	registry := NewModelRegistry(t.TempDir())
	defer registry.CloseModelTasks()
	if !slices.Contains(registry.BrowserProviderIDs(), ai.OpenCodeFreeProviderID) {
		t.Error("OpenCode Free missing from provider navigation")
	}
	expected := map[string]string{
		"big-pickle": "chat", "mimo-v2.6-flash-free": "chat",
		"deepseek-v4.1-flash-free": "chat", "glm-5.3-flash-free": "chat",
		"iceman-free": "chat", "inkling-2-small-free": "chat", "beam-free": "chat",
		"flint-40b-free": "chat", "longcat-2.5-free": "chat", "hy4-flash-free": "chat",
		"nemotron-3-ultra-free": "chat", "nemotron-3.5-super-vl-free": "chat",
		"nemotron-3.5-lightning-free": "chat", "minimax-m3.1-flash-free": "chat",
		"jev-1.13-free": "judge",
	}
	available, all := registry.hubCatalogItems(nil)
	for name, items := range map[string][]tui.ModelSelectorItem{"available": available, "all": all} {
		actual := map[string]string{}
		for _, item := range items {
			if item.Provider == ai.OpenCodeFreeProviderID {
				if _, duplicate := actual[item.ID]; duplicate {
					t.Errorf("duplicate %s model %s", name, item.ID)
				}
				actual[item.ID] = tui.ModelItemKind(item)
			}
		}
		if len(actual) != len(expected) {
			t.Errorf("%s free catalog has %d models, want %d", name, len(actual), len(expected))
		}
		for id, kind := range expected {
			if actual[id] != kind {
				t.Errorf("%s model %s kind = %q, want %q", name, id, actual[id], kind)
			}
		}
	}
}

func TestHubCatalogUsesPublishedAuthWithoutCredentialIO(t *testing.T) {
	registry := NewModelRegistry(t.TempDir())
	defer registry.CloseModelTasks()
	registry.SetCredentialStore(unreadableModelCredentials{t: t})
	registry.SetAvailabilitySnapshot(
		func(provider string) bool { return provider == "openai" || provider == "github-copilot" },
		nil,
		func(string, string) bool { return true },
		func(string, *ai.AuthCheck, func() []string) {},
	)
	_, _ = registry.hubCatalogItems(nil)
}

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

func TestLoginProviderOptionsUsePublishedAuthWithoutCredentialIO(t *testing.T) {
	previous := ai.CredentialStoreFactory
	t.Cleanup(func() { ai.CredentialStoreFactory = previous })
	ai.CredentialStoreFactory = func(string) (ai.ApplicationCredentialStore, error) {
		t.Fatal("login presentation opened credential storage instead of published availability")
		return nil, nil
	}
	registry := NewModelRegistry(t.TempDir())
	defer registry.CloseModelTasks()
	registry.SetAvailabilitySnapshot(
		func(string) bool { return false },
		func(string) (ai.AuthStatus, bool) { return ai.AuthStatus{}, false },
		func(string, string) bool { return false },
		func(string, *ai.AuthCheck, func() []string) {},
	)
	mode := &InteractiveMode{opts: InteractiveModeOptions{AgentDir: t.TempDir(), ModelRegistry: registry}}
	_ = mode.getLoginProviderOptions(true)
}
