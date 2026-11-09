//go:build !pig_strip_todo

// Package todo is ompig-slim's `todo` built-in tool and its stop-time reminder:
// oh-my-pi's task list ported onto PiG's built-in tool registry.
//
// Both halves are native, not extensions. The tool registers itself with
// internal/codingagent/tools from [init], exactly as coding/extension/builtin/websearch
// does, so `todo` is declared with --no-extensions, is active by default (its name
// is in DEFAULT_TOOL_NAMES) and is dropped by --no-builtin-tools. The reminder
// registers a before-settle hook with internal/codingagent, so continuing a run
// whose list still has open work needs no extension either.
//
// The port lives in the vendored tree because a built-in tool is linked code: a
// package outside this module cannot reach internal/codingagent/tools (Go forbids
// importing another module's internal tree), and a vendor package cannot import
// the module that vendors it. websearch and codemode sit here for the same reason.
//
// State lives in each tool result's details, which the session log already stores
// on every call. There is no side table: resuming, rewinding or forking a session
// re-derives the list from the branch exactly as OMP's
// getLatestTodoPhasesFromEntries does, so the tool needs no persistence hooks and
// cannot disagree with the transcript.
package todo

import (
	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
)

// ToolName is the name the model calls.
const ToolName = "todo"

// init registers the built-in where the package is linked, as importing
// upstream's core/tools gives a host its definitions (codemode's and websearch's
// base_tool.go do the same). A host that never assembles the built-in extensions,
// an SDK session or a run with --no-extensions, still has `todo`.
func init() { registerBaseTool() }

// registerBaseTool registers the built-in `todo` definition and the reminder that
// makes it useful. A build that strips todo registers neither.
func registerBaseTool() {
	tools.RegisterExtendedBuiltinTool(ToolName, func(tools.ExtendedBuiltinOptions) tools.ExtendedBuiltinTool {
		return tools.ExtendedBuiltinTool{Definition: Definition()}
	})
	registerSettleHook()
}
