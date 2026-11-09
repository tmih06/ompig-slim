package cli

// Ports packages/coding-agent/src/core/model-resolver.ts

import "github.com/MichaelKinsy/PiG/internal/codingagent"

// ResolveCliModelResult is the shared model-selection result used by CLI and auth commands.
type ResolveCliModelResult = codingagent.ResolveCliModelResult

// ResolveCliModel delegates CLI and auth model selection to the shared resolver.
func ResolveCliModel(cliProvider, cliModel, cliThinking string, inlineAPIKey bool, runtime codingagent.ModelResolverRuntime) ResolveCliModelResult {
	return codingagent.ResolveCliModel(cliProvider, cliModel, cliThinking, inlineAPIKey, runtime)
}
