// Package todotool registers the `todo` built-in: OMP's todo tool, ported as a
// real built-in tool rather than as an extension.
//
// `todo` is a built-in, not an extension: it is created with every session, it
// stays available under --no-extensions, --no-builtin-tools drops it, and it
// never appears in the startup Extensions list. Registering it through
// coding.RegisterExtendedBuiltinTool is what buys that, the same path codemode
// and web_search take. (--help's Built-in Tool Names is a static upstream
// document and does not list codemode or web_search either.)
//
// The tool keeps its whole state in the tool result's details, and re-reads it
// from the session branch on every call, so the list survives resume, rewind and
// fork with no persistence of its own.
package todo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/jsonstringify"
	"github.com/MichaelKinsy/PiG/internal/orderedjson"
)

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

// params is the JSON schema the model sees. It mirrors Params: every
// member but `op` is optional, and a member an op ignores is not an error.
// `op` stays required here because prepareArguments supplies one for the
// payload shapes that pin exactly one op, so `required` still holds a call the
// arguments leave genuinely ambiguous to naming its operation.
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
		// Repairs an omitted `op` before the schema's `required` check sees it. The fork
		// lives here, not only in execute, because validation runs first: a call the
		// arguments already determine would otherwise die as `op: missing` (see
		// prepareArguments).
		PrepareArguments: prepareArguments,
		// The op mutates one shared list, so calls must not interleave.
		ExecutionMode: "sequential",
		Execute:       execute,
		// The card keeps OMP's todo content — the `Todo N tasks` line and the
		// phase tree — but not the tool-card chrome around it. `unframed` drops
		// the border, the status bar and the lifecycle background, so the rows
		// below are all the transcript shows.
		RenderShell:  extension.ToolRenderShellUnframed,
		RenderCall:   renderCall,
		RenderResult: renderResult,
	}
}

// execute applies one op and reports the resulting list.
//
// The previous list is read from the session branch rather than a cache: the
// transcript is the single source of truth, so a resumed or forked session sees
// exactly the list its own branch produced.
func execute(ctx context.Context, _ string, raw json.RawMessage, _ extension.AgentToolUpdateCallback) (extension.AgentToolResult, error) {
	current := currentPhases(ctx)

	var p Params
	if err := json.Unmarshal(raw, &p); err != nil {
		return failure(current, fmt.Sprintf("Invalid todo arguments: %s", err)), nil
	}

	// A view is a read: it must not normalize, rewrite, or report transitions.
	if p.Op == OpView {
		return extension.AgentToolResult{
			Content: []ai.ToolResultMessageContent{textContent(Summary(current, nil, true))},
			Details: Details{Op: OpView, Phases: current},
		}, nil
	}

	next, problems := Apply(ClonePhases(current), p)
	if len(problems) > 0 {
		// Nothing is persisted: the result reports the list as it still is, so a
		// retry cannot collide with a half-applied batch.
		return failure(current, Summary(current, problems, false)), nil
	}

	details := Details{
		Op:             p.Op,
		Phases:         next,
		CompletedTasks: CompletedTransitions(current, next),
	}
	return extension.AgentToolResult{
		Content: []ai.ToolResultMessageContent{textContent(Summary(next, nil, false))},
		Details: details,
	}, nil
}

// failure is a rejected call: it keeps the previous phases in details so the
// next call still reads the truth, and reports the problems as the text.
func failure(current []Phase, message string) extension.AgentToolResult {
	return extension.AgentToolResult{
		Content: []ai.ToolResultMessageContent{textContent(message)},
		Details: Details{Phases: current},
		IsError: true,
	}
}

// prepareArguments supplies an `op` the model left out, before the schema's
// `required` check runs.
//
// The hook is the only place this repair can happen. agent.prepareToolCall calls
// ArgumentPreparer ahead of ai.ValidateToolArgumentsJSON, so an op returned here
// is the op validation and execute both see; the same repair written inside
// execute is unreachable, because validation rejects the call first. (It used to
// live there, and every op-less call died as `op: missing`.)
//
// Models routinely drop `op` from a payload that already determines it, and
// rejecting one costs a whole round trip to fix a shape the arguments name. So
// exactly the shapes that pin one op are repaired:
//
//   - a non-empty `list` is init: init is the only op that reads a list.
//   - a non-empty `items` beside a `phase` is append: append is the only op that
//     requires a phase. Flat init tolerates one too, but a model that names a
//     phase is adding to it rather than restating the plan, and append is the
//     reading that cannot destroy a list it was wrong about.
//   - a non-empty `items` with no `phase` is init: append rejects a missing phase
//     (see appendItems), so init is the only op that can succeed.
//   - an empty object is view: view is the only op that takes no argument at all,
//     so a bare `todo` call can only be asking to see the list. It is read-only,
//     which is what makes the guess safe.
//
// Everything else comes back unchanged and validation reports the missing op.
func prepareArguments(raw json.RawMessage) (json.RawMessage, error) {
	args, err := orderedjson.Parse(raw)
	if err != nil {
		// Not an object: validation reports the arguments it received.
		return raw, nil
	}
	if op, present := args.Get("op"); present && string(op) != "null" {
		// The model named its op; an unknown one is validation's to reject.
		return raw, nil
	}
	// A `null` op is the same omission: the model wrote the member but no operation.
	members := args.Len()
	if _, present := args.Get("op"); present {
		members--
	}
	list, _ := args.Get("list")
	items, _ := args.Get("items")
	phase, _ := args.Get("phase")
	var phaseName string
	_ = json.Unmarshal(phase, &phaseName)
	var op Op
	switch {
	case hasList(list):
		op = OpInit
	case hasList(items) && phaseName != "":
		op = OpAppend
	case hasList(items):
		op = OpInit
	case members == 0:
		op = OpView
	default:
		return raw, nil
	}
	encoded, err := json.Marshal(op)
	if err != nil {
		return raw, nil
	}
	// Assigning args.op appends it after the members the model sent, as a JS object does.
	args.Set("op", encoded)
	out, err := args.MarshalJSON()
	if err != nil {
		return raw, nil
	}
	// JSON.stringify form: no HTML escaping, integer-like members first.
	canonical, err := jsonstringify.Canonicalize(out)
	if err != nil {
		return raw, nil
	}
	return canonical, nil
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
func currentPhases(ctx context.Context) []Phase {
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
