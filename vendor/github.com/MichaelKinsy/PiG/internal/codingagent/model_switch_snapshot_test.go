package codingagent

import (
	"context"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
	"github.com/MichaelKinsy/PiG/tui"
)

type modelSnapshotHandle struct {
	InteractiveSessionHandle
	models       []*ai.Model
	subscription bool
}

func (h *modelSnapshotHandle) AvailableModelSnapshot() []*ai.Model { return h.models }
func (h *modelSnapshotHandle) IsUsingSubscription(string) bool     { return h.subscription }

type unreadableModelCredentials struct {
	ai.CredentialStore
	t *testing.T
}

func (s unreadableModelCredentials) Read(context.Context, string) (*ai.Credential, error) {
	s.t.Fatal("model presentation read credentials instead of published availability")
	return nil, nil
}

type modelSwitchRenderer struct{ tui.TUI }

func (modelSwitchRenderer) Render() {}

func TestSessionModelSwitchReusesRuntimeModel(t *testing.T) {
	for _, selector := range []string{"openai/fixture", "openai/fixture@upstream"} {
		t.Run(selector, func(t *testing.T) {
			target := &ai.Model{ID: "fixture", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai"}}
			settings := NewInMemorySettingsManager(Settings{DefaultModel: "saved", DefaultProvider: "openai"})
			built := false
			mode := &InteractiveMode{
				opts: InteractiveModeOptions{
					SettingsManager: settings,
					ModelLookup: func(provider, id string) *ai.Model {
						if provider == "openai" && id == "fixture" {
							return target
						}
						return nil
					},
					ModelBuilder: func(spec string) (*ai.Model, error) {
						if spec != "openai/fixture@upstream" {
							t.Fatal("catalog selection rebuilt its credential-backed provider")
						}
						built = true
						return target, nil
					},
				},
				statusLine: NewFooterComponent(nil, "", nil),
				tuiInst:    modelSwitchRenderer{},
			}
			finished := false
			mode.switchModel(t.Context(), selector, false, func(err error) {
				finished = true
				if err != nil {
					t.Fatal(err)
				}
			})
			if !finished || mode.opts.Model != target || built != (selector == "openai/fixture@upstream") {
				t.Fatalf("selection finished=%v model=%v built=%v", finished, mode.opts.Model, built)
			}
			if settings.GetDefaultModel() != "saved" {
				t.Fatal("session selection overwrote the saved default")
			}
		})
	}
}

func TestSessionModelItemsUsePublishedAvailability(t *testing.T) {
	registry := NewModelRegistry(t.TempDir())
	registry.SetCredentialStore(unreadableModelCredentials{t: t})
	visible := &ai.Model{ID: "visible", DisplayName: "Visible", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai"}}
	hidden := &ai.Model{ID: "hidden", ProviderMeta: ai.ProviderMetadata{ProviderID: "anthropic"}}
	handle := &modelSnapshotHandle{models: []*ai.Model{visible, hidden}}
	settings := NewInMemorySettingsManager(Settings{})
	settings.ApplyOverrides(Settings{Config: modelroles.Config{DisabledProviders: []string{"anthropic"}}})
	mode := &InteractiveMode{opts: InteractiveModeOptions{SessionHandle: handle, SettingsManager: settings, ModelRegistry: registry}}
	items := mode.availableModelItems()
	if len(items) != 1 || items[0].FQ() != "openai/visible" || items[0].Model != visible {
		t.Fatalf("picker items = %+v, want only the published visible model", items)
	}
	handle.models = nil
	if got := mode.availableModelItems(); len(got) != 0 {
		t.Fatalf("empty published availability fell back to another catalog: %+v", got)
	}
}

func TestSessionModelSubscriptionUsesPublishedAuth(t *testing.T) {
	registry := NewModelRegistry(t.TempDir())
	registry.SetCredentialStore(unreadableModelCredentials{t: t})
	handle := &modelSnapshotHandle{}
	mode := &InteractiveMode{opts: InteractiveModeOptions{SessionHandle: handle, ModelRegistry: registry}}
	model := &ai.Model{ID: "fixture", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai"}}
	for _, subscribed := range []bool{true, false} {
		handle.subscription = subscribed
		if got := mode.footerUsingSubscription(model); got != subscribed {
			t.Fatalf("subscription marker = %v, want published value %v", got, subscribed)
		}
	}
	model.ProviderMeta.ProviderID = "kimi-coding"
	if !mode.footerUsingSubscription(model) {
		t.Fatal("Kimi lost its API-key subscription marker")
	}
}
