//go:build !pig_strip_websearch

// Package websearch registers the built-in `web_search` tool.
//
// The tool is a built-in, not an extension: it is declared with --no-extensions
// and active by default (its name is in DEFAULT_TOOL_NAMES). Its implementation
// lives in internal/webtools, which this package may import because the reverse
// edge would cycle: internal/codingagent/tools imports internal/codingagent.
package websearch

import (
	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
)

// ToolName is the name the model calls.
const ToolName = "web_search"

func init() {
	tools.RegisterExtendedBuiltinTool(ToolName, func(options tools.ExtendedBuiltinOptions) tools.ExtendedBuiltinTool {
		return tools.ExtendedBuiltinTool{Definition: Definition(options)}
	})
}
