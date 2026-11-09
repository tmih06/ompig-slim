package coding

// Ports the part of packages/coding-agent/src/core/tools/index.ts that a host needs to add a built-in tool: createAllTools
// and the definition registry it reads.

import (
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
)

// ExtendedBuiltinOptions are the per-session inputs an extended built-in definition is built with.
// It is [tools.ExtendedBuiltinOptions], re-exported so a program outside this module can write an extended built-in.
type ExtendedBuiltinOptions = tools.ExtendedBuiltinOptions

// ExtendedBuiltinTool is one extended built-in: its full definition, which a session registers as a built-in tool.
type ExtendedBuiltinTool = tools.ExtendedBuiltinTool

// ExtendedBuiltinFactory builds an extended built-in definition for a session.
type ExtendedBuiltinFactory = tools.ExtendedBuiltinFactory

// RegisterExtendedBuiltinTool records the factory of a built-in tool, replacing an earlier one of that name. A registered
// tool is a built-in, not an extension: it is created with every session, it stays available under --no-extensions, it
// appears in --help's Built-in Tool Names, and --no-builtin-tools drops it. It is active at startup only when a default
// selection names it: the built-in default set, the `defaultTools` setting, or --tools.
//
// Call it from an init of the package that owns the definition, or from the program's main before it builds a session.
// Registering after a session was built leaves that session on the definition it already created.
//
// pig additive: upstream's core/tools package exports its definitions directly, so a host imports the one it needs.
// PiG's extended built-ins live in internal/codingagent/tools, which a program outside this module cannot import; this
// re-export is the seam it registers through.
func RegisterExtendedBuiltinTool(name string, factory ExtendedBuiltinFactory) {
	tools.RegisterExtendedBuiltinTool(name, factory)
}

// IsExtendedBuiltinTool reports whether name is a registered built-in tool.
func IsExtendedBuiltinTool(name string) bool { return tools.IsExtendedBuiltinTool(name) }

// ExtendedBuiltinDefinition builds the definition of the built-in tool name, or the zero definition when no build
// registered it. A session that registers the tool reads this definition, not the projection the agent tool interface
// carries: prepareLoadout, exposure, reachesTools and the output schema live only on the definition.
func ExtendedBuiltinDefinition(name string, options ExtendedBuiltinOptions) extension.ToolDefinition {
	return tools.ExtendedBuiltinDefinition(name, options)
}
