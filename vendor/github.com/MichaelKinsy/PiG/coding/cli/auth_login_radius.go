//go:build !pig_strip_mcp

package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/mcpext"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
)

func configureAuthRadiusMCP(provider *ai.ModelsProvider, env authRunEnv, terminal *authLoginTerminal) {
	cwd, _ := os.Getwd()
	plan, err := codingagent.PlanRadiusMcpServer(provider.ID, env.agentDir, cwd)
	if err != nil {
		env.printError(fmt.Sprintf("Could not update %s: %v", filepath.Join(env.agentDir, "mcp.json"), err))
		return
	}
	if plan == nil {
		return
	}
	replaced, err := mcpext.AddMcpServerConfigJSON(plan.Path, plan.Name, plan.Config)
	if err != nil {
		env.printError(fmt.Sprintf("Could not update %s: %v", plan.Path, err))
		return
	}
	verb := "Added"
	if replaced {
		verb = "Updated"
	}
	terminal.write(fmt.Sprintf("%s MCP server %q in %s to use the %s login.", verb, plan.Name, plan.Path, provider.Name))
}
