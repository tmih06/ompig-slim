//go:build !pig_strip_codemode

package codemode

// codemode is a built-in tool, not an extension: upstream moved the sandbox, renderers and worker into core/tools
// (packages/coding-agent/src/core/tools/codemode), so `codemode` is declared with --no-extensions and active by default.
//
// Go mechanic (not a divergence): PiG's implementation stays in this package because its sandbox depends on the codemode
// engine, and this package imports internal/codingagent, which imports internal/codingagent/tools. The definition
// therefore registers itself with that package's extended-built-in registry, which CreateAllTools and the session tool
// registry read (upstream's createAllToolDefinitions names createCodemodeToolDefinition directly).
//
// The extension factory ([CreateCodemodeExtension]) still exists for SDK extensions that name createCodemodeExtension; a
// session that registers it replaces this base tool with the same definition.

import "github.com/MichaelKinsy/PiG/internal/codingagent/tools"

// registerBaseTool registers the built-in `codemode` definition. A build that strips codemode registers nothing.
func registerBaseTool() {
	tools.RegisterExtendedBuiltinTool(ToolName, func(options tools.ExtendedBuiltinOptions) tools.ExtendedBuiltinTool {
		built := Options{Models: true, CacheDir: options.CacheDir, GetSettings: options.Settings}
		// Active on registration, like upstream's built-in definition: DEFAULT_TOOL_NAMES names it (settings-manager.ts:89).
		return tools.ExtendedBuiltinTool{Definition: Definition(built)}
	})
}

// init registers the base tool where the package is linked, as importing core/tools gives upstream the definition: a host
// that never assembles the built-in extensions (an SDK session, or a run with --no-extensions) still has codemode. The
// tool package drops a registration its runtime strip list names, so a build without codemode registers nothing.
func init() { registerBaseTool() }
