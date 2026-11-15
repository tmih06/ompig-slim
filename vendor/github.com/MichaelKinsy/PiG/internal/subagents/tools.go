package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

var ToolNames = []string{"Agent", "get_subagent_result", "steer_subagent", "SubagentWorkflow"}

func IsTool(name string) bool {
	for _, n := range ToolNames {
		if n == name {
			return true
		}
	}
	return false
}

type Tool struct {
	Manager                     *Manager
	ToolName, Owner, WorkflowID string
}

func (t *Tool) Name() string { return t.ToolName }
func (t *Tool) Label() string {
	switch t.ToolName {
	case "Agent":
		return "Agent"
	case "get_subagent_result":
		return "Get Agent Result"
	case "steer_subagent":
		return "Steer Agent"
	}
	return t.ToolName
}
func (t *Tool) ExecutionMode() agent.ToolExecutionMode { return agent.ToolModeParallel }
func property(kind, description string) map[string]any {
	return map[string]any{"type": kind, "description": description}
}
func (t *Tool) Schema() ai.ToolSchema {
	props := map[string]any{}
	required := []string{}
	description := ""
	s := t.Manager.Settings()
	switch t.ToolName {
	case "Agent":
		description = t.Manager.toolDescription(false)
		role := map[string]any{"type": "string", "minLength": 2, "description": "Required configured model role, such as @task."}
		if roles := t.Manager.ModelRoles(); len(roles) > 0 {
			role["enum"] = roles
		}
		props = map[string]any{"prompt": property("string", "The task for the agent to perform."), "description": property("string", "Short (3-5 word) task description shown in UI."), "role": role, "tool": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "minLength": 1}, "description": "Required list of tool names the agent may use."}, "name": property("string", "Optional memorable handle (letters, digits, underscores and hyphens)."), "max_turns": map[string]any{"type": "integer", "minimum": 1, "maximum": 10000, "description": "Soft turn limit, followed by a wrap-up warning and grace turns."}, "run_in_background": property("boolean", "Default true. False waits inline; ctrl+b sends a running foreground agent to the background."), "resume": property("string", "Resume an agent ID or handle after its current run finishes, preserving conversation."), "isolated": property("boolean", "Only built-in tools; omit extension and MCP tools."), "inherit_context": property("boolean", "Fork parent conversation into the agent. Default false.")}
		if s.WorktreeIsolation {
			props["isolation"] = map[string]any{"type": "string", "enum": []string{"off", "worktree"}, "description": "off (default) uses current checkout; worktree creates an isolated copy at HEAD, without uncommitted main-checkout changes."}
		}
		if s.SchedulingEnabled && t.Owner == "" {
			props["schedule"] = property("string", "Six-field cron, recurring interval (5m), relative one-shot (+10m), or ISO timestamp. Cannot combine with resume, inherit_context or foreground.")
		}
		required = []string{"prompt", "description", "role", "tool"}
	case "get_subagent_result":
		description = "Retrieve the agent's exact final result. Short notifications already contain a preview. Use wait:true when completion is required; do not poll."
		props = map[string]any{"agent_id": property("string", "Agent ID or handle."), "wait": property("boolean", "Wait for completion. Default false.")}
		required = []string{"agent_id"}
	case "steer_subagent":
		description = "Send a message to a running agent. Delivered after the current tool execution as a user message, without restarting."
		props = map[string]any{"agent_id": property("string", "Agent ID or handle."), "message": property("string", "Steering message.")}
		required = []string{"agent_id", "message"}
	case "SubagentWorkflow":
		description = t.Manager.toolDescription(true)
		props = map[string]any{"script": property("string", "Inline JavaScript beginning with export const meta = { name, description, phases? }."), "scriptPath": property("string", "Workflow file path, absolute or relative to project; takes precedence over script."), "name": property("string", "Saved workflow name in .pi/workflows, .agents/workflows or the agent directory."), "args": map[string]any{"description": "JSON value passed verbatim as args."}, "resumeFromRunId": property("string", "Replay unchanged successful leading calls from this run."), "title": property("string", "Ignored; use meta.name."), "description": property("string", "Ignored; use meta.description.")}
	}
	return ai.ToolSchema{Name: t.Name(), Description: description, Parameters: map[string]any{"type": "object", "properties": props, "required": required}}
}
func textResult(text string, details any) agent.AgentToolResult {
	return agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: text}}, Details: details}
}

// toolResultPreview bounds model-visible completion text while keeping the full
// result in Record for explicit retrieval and persistence.
func toolResultPreview(text string) string {
	return clip(text, maxToolResultPreview)
}
func (t *Tool) Execute(ctx context.Context, callID string, raw json.RawMessage, update agent.ToolUpdateCallback) (result agent.AgentToolResult, err error) {
	defer func() {
		if err == nil {
			result.Usage = t.Manager.DrainUsage(t.Owner)
		}
	}()
	if !t.Manager.Settings().Enabled {
		return agent.AgentToolResult{}, fmt.Errorf("subagents are disabled; enable with /subagents on")
	}
	switch t.ToolName {
	case "Agent":
		var p Params
		if err := json.Unmarshal(raw, &p); err != nil {
			return agent.AgentToolResult{}, err
		}
		if strings.TrimSpace(p.Role) == "" || len(p.Tools) == 0 {
			return agent.AgentToolResult{}, fmt.Errorf("role and tool are required")
		}
		for _, name := range p.Tools {
			if strings.TrimSpace(name) == "" {
				return agent.AgentToolResult{}, fmt.Errorf("tool names must not be empty")
			}
		}
		p.Model = p.Role
		p.Type = ""
		r, err := t.Manager.Spawn(ctx, p, t.Owner, t.WorkflowID)
		if err != nil {
			return agent.AgentToolResult{}, err
		}
		if r.Status == "scheduled" {
			return textResult(r.Result, r), nil
		}
		if r.Background {
			return textResult(backgroundResult(r), r), nil
		}
		waitCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		done := make(chan struct{})
		updatesDone := make(chan struct{})
		agentID := r.ID
		if update != nil {
			go func() {
				defer close(updatesDone)
				ticker := time.NewTicker(200 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-done:
						return
					case <-waitCtx.Done():
						return
					case <-ticker.C:
						if current, e := t.Manager.Consume(agentID, t.Owner); e == nil {
							update(textResult("", current))
						}
					}
				}
			}()
		} else {
			close(updatesDone)
		}
		r, err = t.Manager.Wait(ctx, r.ID, t.Owner, true)
		cancel()
		close(done)
		<-updatesDone
		if err != nil {
			return agent.AgentToolResult{}, err
		}
		if r.Active() {
			return textResult(backgroundResult(r), r), nil
		}
		result := textResult(toolResultPreview(r.Result), r)
		result.IsError = r.Status == "error" || r.Status == "aborted"
		if r.Error != "" && r.Error != r.Result {
			result.Content = append(result.Content, ai.TextContent{Text: toolResultPreview(r.Error)})
		}
		return result, nil
	case "get_subagent_result":
		var p struct {
			ID   string `json:"agent_id"`
			Wait bool
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return agent.AgentToolResult{}, err
		}
		var r Record
		var err error
		if p.Wait {
			r, err = t.Manager.Wait(ctx, p.ID, t.Owner, false)
		} else {
			r, err = t.Manager.Consume(p.ID, t.Owner)
		}
		if err != nil {
			if wf, ok := t.Manager.Workflow(p.ID); ok {
				if p.Wait || !wf.Active() {
					var waitErr error
					wf, waitErr = t.Manager.WaitWorkflow(ctx, p.ID)
					if waitErr != nil {
						return agent.AgentToolResult{}, waitErr
					}
				}
				return textResult(wf.ResultText(), wf), nil
			}
			return agent.AgentToolResult{}, err
		}
		return textResult(r.Result, r), nil
	case "steer_subagent":
		var p struct {
			ID      string `json:"agent_id"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return agent.AgentToolResult{}, err
		}
		if err := t.Manager.Steer(p.ID, p.Message, t.Owner); err != nil {
			return agent.AgentToolResult{}, err
		}
		return textResult("Steering message sent to agent "+p.ID, nil), nil
	case "SubagentWorkflow":
		var p WorkflowParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return agent.AgentToolResult{}, err
		}
		wf, err := t.Manager.StartWorkflow(ctx, p)
		if err != nil {
			return agent.AgentToolResult{}, err
		}
		return textResult(fmt.Sprintf("Workflow %s started in background.\nScript: %s\nYou will be notified on completion.", wf.ID, wf.ScriptPath), wf), nil
	}
	return agent.AgentToolResult{}, fmt.Errorf("unknown subagent tool %q", t.ToolName)
}
func backgroundResult(r Record) string {
	return fmt.Sprintf("Agent launched in background.\nagent_id: %s\nhandle: @%s\noutput_file: %s\nYou will be notified on completion. Continue other work; use get_subagent_result for the full result or steer_subagent to redirect it.", r.ID, r.Handle, r.OutputFile)
}
