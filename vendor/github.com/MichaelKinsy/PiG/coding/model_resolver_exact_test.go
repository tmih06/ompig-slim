package coding

import (
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
)

// TestExactCliResolutionMatchesCatalog checks the exact path against the full
// resolver, including thinking suffixes and fuzzy misses which must fall back.
func TestExactCliResolutionMatchesCatalog(t *testing.T) {
	session := roleTestSession(t, nil)
	runtime := session.modelRuntime
	for _, id := range []string{"gpt-4o", "gpt-4o:high", "gpt-4o:off", "gpt-4o-mini", "unknown-fixture-model"} {
		for _, thinking := range []ai.ThinkingLevel{"", "off", "high"} {
			want := icodingagent.ResolveCliModel("openai", id, string(thinking), true, cliResolverRuntime{runtime})
			got := ResolveCliModel(ResolveCliModelOptions{CliProvider: "openai", CliModel: id, CliThinking: thinking, InlineApiKey: true, ModelRuntime: runtime})
			if got.Error != want.Error || got.Warning != want.Warning || got.ThinkingLevel != want.ThinkingLevel {
				t.Fatalf("%s/%s: diagnostics differ: %+v vs %+v", id, thinking, got, want)
			}
			if want.Model != nil {
				source := want.Model.SourceModel()
				if got.Model == nil || source.ID != got.Model.ID || source.ProviderMeta.ProviderID != got.Model.ProviderMeta.ProviderID || source.ProviderMeta.API != got.Model.ProviderMeta.API {
					t.Fatalf("%s/%s: selected model differs", id, thinking)
				}
			}
		}
	}
}
