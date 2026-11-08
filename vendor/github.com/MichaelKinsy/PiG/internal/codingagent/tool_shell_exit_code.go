package codingagent

// Ports the facts OMP's shell renderer reads from a result that the Go tool
// boundary does not carry in its details:
//
//	packages/tui/src/tools/bash.ts  BashToolDetails.exitCode, shellToolHead

import (
	"encoding/json"

	"github.com/MichaelKinsy/PiG/agent"
)

// shellExitCode reads a completed shell command's exit status from its
// structured content (upstream BashToolOutput.exit_code). Ompig's in-process
// BashDetails carries no exit code, so the model-facing object is the source.
// It returns nil for a missing, malformed, or non-integer field rather than
// drawing a code the process never reported.
func shellExitCode(result agent.AgentToolResult) *int {
	if len(result.StructuredContent) == 0 {
		return nil
	}
	var content struct {
		ExitCode *json.Number `json:"exit_code"`
	}
	if json.Unmarshal(result.StructuredContent, &content) != nil || content.ExitCode == nil {
		return nil
	}
	code, err := content.ExitCode.Int64()
	if err != nil {
		return nil
	}
	value := int(code)
	return &value
}
