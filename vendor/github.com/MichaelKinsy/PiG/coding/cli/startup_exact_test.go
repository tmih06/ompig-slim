package cli

import (
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
)

// TestExactStartupLookupDoesNotListCatalog guards saved-default, role and resume
// resolution. An exact miss must not trigger unrelated provider decoding either.
func TestExactStartupLookupDoesNotListCatalog(t *testing.T) {
	model := &ai.Model{ID: "fixture", ProviderMeta: ai.ProviderMetadata{ProviderID: "local"}}
	runtime := &startupModelRuntime{
		load: func() []codingagent.RuntimeModel { t.Fatal("exact lookup listed the full catalog"); return nil },
		lookup: func(provider, id string) *ai.Model {
			if provider == "local" && id == "fixture" {
				return model
			}
			return nil
		},
	}
	if got := runtime.getModel("local", "fixture"); got == nil || got.NativeModel != model {
		t.Fatal("exact lookup lost the canonical model", got)
	}
	if got := runtime.getModel("local", "missing"); got != nil {
		t.Fatal("exact miss resolved", got)
	}
}
