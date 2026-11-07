package cli

import (
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/coding/extension/builtin"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
)

// llamaBuiltinName is the name of the built-in llama.cpp extension, the first entry of builtInExtensions.
// Ports .upstream/v0.99.2/packages/coding-agent/src/extensions/index.ts:8.
const llamaBuiltinName = "llama.cpp"

// nativeBuiltInExtensions are the built-in extensions of the CLI, in extensions/index.ts order: llama.cpp (whose provider the llama
// host supplies), tool-search and mcp. settings supplies the agent directory of `mcp.json` on every use; nil means the defaults.
// codemode is not an extension any more: it is a built-in tool registered by [builtin.All] alongside these extensions, so it
// stays available with --no-extensions and is active by default.
// Ports .upstream/v0.99.2/packages/coding-agent/src/extensions/index.ts:7-14, after beb830782.
func nativeBuiltInExtensions(settings *codingagent.SettingsManager) []extension.InlineExtension {
	var options builtin.Options
	// The codemode sandbox keeps wazero's compilation cache; the built-in tool reads it through the tool options.
	options.ConfigureCodemode(tools.WazeroCacheDir())
	var agentDir string
	if settings != nil {
		agentDir = settings.AgentDir()
	} else if dir, err := codingagent.ResolveAgentDir(); err == nil {
		// The package-initialization list (builtInExtensions) is built before main can report an unresolvable home directory; it is the name list, and the session's list is built from the session's settings.
		agentDir = dir
	}
	options.ConfigureMcp(agentDir, codingagent.ConfigDirName(), codingagent.OpenBrowser, codingagent.CopyToClipboard)
	// pig additive (D202): a closed Piglet Binary starts no server from the user or project mcp.json.
	options.RefuseMcpConfigFiles(pigletBinaryMcpConfigRefusal(currentPigletBinary()))
	// pig additive (D92): a Binary without llama.cpp has no llama.cpp entry.
	inline := llamaInlineExtensions()
	for _, entry := range builtin.All(options) {
		inline = append(inline, extension.NamedInlineExtension{Name: entry.Name, Factory: entry.Factory, Replaceable: entry.Replaceable, Builtin: true})
	}
	return inline
}
