// Package todotool registers the `todo` built-in: OMP's todo tool, ported onto
// the extension API so it ships as part of ompig-slim without patches to the
// vendored runtime.
//
// The tool keeps its whole state in the tool result's details, and re-reads it
// from the session branch on every call, so the list survives resume, rewind and
// fork with no persistence of its own.
package todotool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"

	"ompig-slim/internal/todo"
)

// ToolName is the name the model calls.
const ToolName = "todo"

// description is the model-facing prompt. It carries only the rules a model
// gets wrong without them: one op per call, riding along with the next real
// tool call, addressing tasks by verbatim content, and closing tasks as they
// finish instead of after a batch. Everything else the tool can enforce itself
// — through its result text and the stop-time reminder — so it is not spent on
// every request.
const description = "Track multi-step work in a session todo list. One op per call; never call it alone — " +
	"emit it beside your next real tool call. `init` when work has 3+ steps, has phases, or the user listed " +
	"several items: list EVERY item separately. Address tasks by their verbatim content, never by an invented " +
	"id. Mark each task done the moment it is done. When work waits on the user, an agent or a service, " +
	"`block` it with a reason instead of stopping."

// params is the JSON schema the model sees. It mirrors todo.Params: every
// member but `op` is optional, and a member an op ignores is not an error.
const params = `{"type":"object","required":["op"],"properties":{
"op":{"type":"string","enum":["init","start","done","drop","block","unblock","rm","append","view"],"description":"Operation to apply."},
"list":{"type":"array","description":"init: phases to create; replaces the list.","items":{"type":"object","required":["phase","items"],"properties":{"phase":{"type":"string"},"items":{"type":"array","items":{"type":"string"}}}}},
"items":{"type":"array","items":{"type":"string"},"description":"init: tasks for one phase named by phase. append: tasks to add to phase."},
"phase":{"type":"string","description":"Phase name. Required by append; target for done/drop/block/unblock/rm."},
"task":{"type":"string","description":"Verbatim task content. Target for start/done/drop/block/unblock/rm."},
"reason":{"type":"string","description":"block: one-line note on what the task waits for."}}}`

// Definition builds the tool. It is active by default, like OMP's.
func Definition() extension.ToolDefinition {
	return extension.ToolDefinition{
		Name:        ToolName,
		Label:       "Todo",
		Description: description,
		// One short line in the system prompt's tool list. Deliberately not a
		// longer snippet: the tool is self-describing at the point of use.
		PromptSnippet: "Track multi-step work in a session todo list",
		Parameters:    json.RawMessage(params),
		// The op mutates one shared list, so calls must not interleave.
		ExecutionMode: "sequential",
		Execute:       execute,
		// The card is drawn by OMP's todo renderer (render.go). Without these the
		// tool falls back to the generic card, which shows the raw summary text.
		RenderCall:   renderCall,
		RenderResult: renderResult,
	}
}

// Extension registers the tool. It is the factory cli.Main composes in, so the
// tool exists in every session that loads the program's extensions.
func Extension() extension.ExtensionFactory {
	return func(api extension.API) error {
		api.RegisterTool(Definition())
		return nil
	}
}

// execute applies one op and reports the resulting list.
//
// The previous list is read from the session branch rather than a cache: the
// transcript is the single source of truth, so a resumed or forked session sees
// exactly the list its own branch produced.
func execute(ctx context.Context, _ string, raw json.RawMessage, _ extension.AgentToolUpdateCallback) (extension.AgentToolResult, error) {
	current := currentPhases(ctx)

	var p todo.Params
	if err := json.Unmarshal(raw, &p); err != nil {
		return failure(current, fmt.Sprintf("Invalid todo arguments: %s", err)), nil
	}
	if repaired, ok := inferOp(raw, p, len(current) > 0); ok {
		p = repaired
	}

	// A view is a read: it must not normalize, rewrite, or report transitions.
	if p.Op == todo.OpView {
		return extension.AgentToolResult{
			Content: []ai.ToolResultMessageContent{textContent(todo.Summary(current, nil, true))},
			Details: todo.Details{Op: todo.OpView, Phases: current},
		}, nil
	}

	next, problems := todo.Apply(todo.ClonePhases(current), p)
	if len(problems) > 0 {
		// Nothing is persisted: the result reports the list as it still is, so a
		// retry cannot collide with a half-applied batch.
		return failure(current, todo.Summary(current, problems, false)), nil
	}

	details := todo.Details{
		Op:             p.Op,
		Phases:         next,
		CompletedTasks: todo.CompletedTransitions(current, next),
	}
	return extension.AgentToolResult{
		Content: []ai.ToolResultMessageContent{textContent(todo.Summary(next, nil, false))},
		Details: details,
	}, nil
}

// failure is a rejected call: it keeps the previous phases in details so the
// next call still reads the truth, and reports the problems as the text.
func failure(current []todo.Phase, message string) extension.AgentToolResult {
	return extension.AgentToolResult{
		Content: []ai.ToolResultMessageContent{textContent(message)},
		Details: todo.Details{Phases: current},
		IsError: true,
	}
}

// inferOp repairs an omitted `op` when the payload can only mean one operation.
//
// Models routinely send `{list:[...]}` or `{items:[...]}` with no op. Rejecting
// that costs a whole round trip to fix a shape the arguments already determine,
// so the unambiguous cases are repaired here and everything else is left to the
// normal validation error.
func inferOp(raw json.RawMessage, p todo.Params, hasExisting bool) (todo.Params, bool) {
	if p.Op != "" {
		return p, false
	}
	var probe struct {
		List  json.RawMessage `json:"list"`
		Items json.RawMessage `json:"items"`
		Phase string          `json:"phase"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		return p, false
	}
	switch {
	case hasList(probe.List):
		p.Op = todo.OpInit
	case hasList(probe.Items) && probe.Phase != "":
		p.Op = todo.OpAppend
	case hasList(probe.Items) && !hasExisting:
		// Nothing to overwrite, so a bare items list can only be an init.
		p.Op = todo.OpInit
	default:
		return p, false
	}
	return p, true
}

// hasList reports whether raw is a non-empty JSON array.
func hasList(raw json.RawMessage) bool {
	var items []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &items) != nil {
		return false
	}
	return len(items) > 0
}

// currentPhases reads the list from the latest successful state-changing todo
// result on the session branch. A `view` result is skipped because it echoes
// state it did not change; an errored result is skipped because it persisted
// nothing.
//
// Returns an empty list outside a session (a direct unit call), which behaves as
// "no todos yet".
func currentPhases(ctx context.Context) []todo.Phase {
	tc := extension.ToolContextFromContext(ctx)
	if tc == nil {
		return nil
	}
	manager, err := tc.SessionManager()
	if err != nil || manager == nil {
		return nil
	}
	branch := manager.GetBranch()
	for i := len(branch) - 1; i >= 0; i-- {
		phases, ok := phasesOf(branch[i])
		if !ok {
			continue
		}
		return phases
	}
	return nil
}

// textContent wraps text in the one result block the model reads.
func textContent(text string) ai.ToolResultMessageContent {
	return ai.TextContent{Text: text}
}
