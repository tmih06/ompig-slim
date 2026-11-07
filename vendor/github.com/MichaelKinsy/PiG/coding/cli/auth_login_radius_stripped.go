//go:build pig_strip_mcp

package cli

import "github.com/MichaelKinsy/PiG/ai"

// A build that deliberately omits MCP must not offer or install MCP servers.
func configureAuthRadiusMCP(*ai.ModelsProvider, authRunEnv, *authLoginTerminal) {}
