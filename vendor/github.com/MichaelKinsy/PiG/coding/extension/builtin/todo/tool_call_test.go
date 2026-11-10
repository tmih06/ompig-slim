package todo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
)

// TestUnparseableArgumentsReportTheParseError pins the message a model reads when
// its own arguments text was not JSON.
//
// The observed failure: a model emitted `todo` with a garbled argument string, the
// parser could not read it, and the model was told `op: missing` against `{}` — a
// report about a payload it never sent. The call carries the empty object because
// there is no reading of the text, so validating it names whatever required member
// the schema lists first and buries the real cause. The parse error must reach the
// model instead, because that is the only text it can act on.
func TestUnparseableArgumentsReportTheParseError(t *testing.T) {
	definition := Definition()
	tool := tools.DefinitionAgentTool(definition)
	// A call whose arguments text did not parse: ai's parser leaves Arguments empty
	// and records why, which is what reaches the agent as a pending call.
	call := ai.ParseToolCallArguments(`{"init": list" /><parameter name="phase">investigate}`)
	if call.ArgumentsError == "" {
		t.Fatal("the fixture arguments must fail to parse")
	}
	call.ID, call.Name = "call-1", ToolName

	outcome, err := agent.RunToolCall(t.Context(), call, agent.RunToolCallOptions{
		Tools:    []agent.AgentTool{tool},
		OnUpdate: func(agent.AgentToolResult) error { return nil },
	})
	if err != nil {
		t.Fatalf("RunToolCall: %v", err)
	}
	if !outcome.IsError {
		t.Fatal("a call with unparseable arguments must fail")
	}
	message := outcome.Result.Text()
	if !strings.Contains(message, "Invalid arguments for tool \"todo\"") {
		t.Fatalf("the result must name the parse failure, got:\n%s", message)
	}
	if strings.Contains(message, "op: missing") {
		t.Fatalf("the result must not report a missing member of a payload the model never sent, got:\n%s", message)
	}
}

// TestOmittedOpReachesTheTool drives an op-less payload through the same entry
// point the agent loop uses, so the repair is proven on the path the failure took
// rather than on prepareArguments alone. Validation runs inside RunToolCall; a
// repair that only satisfied it while execute saw nothing would still fail here.
func TestOmittedOpReachesTheTool(t *testing.T) {
	definition := Definition()
	tool := tools.DefinitionAgentTool(definition)
	if definition.PrepareArguments == nil {
		t.Fatal("the todo definition must repair arguments before validation")
	}

	var call ai.ToolCall
	if err := call.SetArgumentsJSON([]byte(`{"list":[{"phase":"P","items":["a","b"]}]}`)); err != nil {
		t.Fatalf("SetArgumentsJSON: %v", err)
	}
	call.ID, call.Name = "call-2", ToolName

	outcome, err := agent.RunToolCall(t.Context(), call, agent.RunToolCallOptions{
		Tools:    []agent.AgentTool{tool},
		OnUpdate: func(agent.AgentToolResult) error { return nil },
	})
	if err != nil {
		t.Fatalf("RunToolCall: %v", err)
	}
	if outcome.IsError {
		t.Fatalf("an op-less init must be repaired, not rejected: %s", outcome.Result.Text())
	}
	details, ok := outcome.Result.Details.(Details)
	if !ok {
		t.Fatalf("details are %T, want todo.Details", outcome.Result.Details)
	}
	if details.Op != OpInit {
		t.Fatalf("op = %q, want init", details.Op)
	}
	if len(details.Phases) != 1 || len(details.Phases[0].Tasks) != 2 {
		t.Fatalf("the init did not reach the list: %+v", details.Phases)
	}
}

// TestBareCallReadsTheList is the case behind the reported error: a model that
// calls `todo` with nothing at all is asking to see the list.
func TestBareCallReadsTheList(t *testing.T) {
	tool := tools.DefinitionAgentTool(Definition())
	var call ai.ToolCall
	if err := call.SetArgumentsJSON([]byte(`{}`)); err != nil {
		t.Fatalf("SetArgumentsJSON: %v", err)
	}
	call.ID, call.Name = "call-3", ToolName

	outcome, err := agent.RunToolCall(t.Context(), call, agent.RunToolCallOptions{
		Tools:    []agent.AgentTool{tool},
		OnUpdate: func(agent.AgentToolResult) error { return nil },
	})
	if err != nil {
		t.Fatalf("RunToolCall: %v", err)
	}
	if outcome.IsError {
		t.Fatalf("a bare todo call must read the list, not fail: %s", outcome.Result.Text())
	}
	if details := outcome.Result.Details.(Details); details.Op != OpView {
		t.Fatalf("op = %q, want a view", details.Op)
	}
}

// TestOmittedOpPayloadKeepsItsMembers guards the repair against dropping what the
// model sent: the repair adds `op` to the arguments, it does not rebuild them.
func TestOmittedOpPayloadKeepsItsMembers(t *testing.T) {
	prepared, err := prepareArguments(json.RawMessage(`{"phase":"P","items":["a","b"]}`))
	if err != nil {
		t.Fatalf("prepareArguments: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(prepared, &decoded); err != nil {
		t.Fatalf("the repaired arguments are not JSON: %v", err)
	}
	if decoded["op"] != "append" || decoded["phase"] != "P" {
		t.Fatalf("the repair lost members: %s", prepared)
	}
	if items, ok := decoded["items"].([]any); !ok || len(items) != 2 {
		t.Fatalf("the repair lost items: %s", prepared)
	}
}

// TestObservedCallShapesReachTheTool replays the payloads that failed in real
// sessions through the agent's own call path, so each is proven to become a
// mutation rather than a `Validation failed for tool "todo"` result.
//
// The shapes come from this project's session logs: the op written as a member
// (`{"init": [...]}`) both as an array and as its JSON text, an op of null or "",
// and a bare `{}` call. The last case below is the part the repair must not do:
// a payload that names no operation and no arguments still has to be rejected
// with a message the model can act on, and must not be shown as a dump of its
// own argument text.
func TestObservedCallShapesReachTheTool(t *testing.T) {
	tool := tools.DefinitionAgentTool(Definition())
	phases := `[{"phase":"Verify","items":["Check the finding"]}]`
	cases := map[string]struct {
		args   string
		wantOp Op
	}{
		"op name as the member":             {args: `{"init":` + phases + `}`, wantOp: OpInit},
		"op name holding the array as text": {args: `{"init":"` + strings.ReplaceAll(phases, `"`, `\"`) + `"}`, wantOp: OpInit},
		"null op beside a list":             {args: `{"op":null,"list":` + phases + `}`, wantOp: OpInit},
		"empty op beside items":             {args: `{"op":"","items":["a"]}`, wantOp: OpInit},
		"no members at all":                 {args: `{}`, wantOp: OpView},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var call ai.ToolCall
			if err := call.SetArgumentsJSON([]byte(c.args)); err != nil {
				t.Fatalf("SetArgumentsJSON(%s): %v", c.args, err)
			}
			call.ID, call.Name = "call", ToolName
			outcome, err := agent.RunToolCall(t.Context(), call, agent.RunToolCallOptions{
				Tools:    []agent.AgentTool{tool},
				OnUpdate: func(agent.AgentToolResult) error { return nil },
			})
			if err != nil {
				t.Fatalf("RunToolCall: %v", err)
			}
			if outcome.IsError {
				t.Fatalf("%s must be repaired, not rejected: %s", c.args, outcome.Result.Text())
			}
			if details := outcome.Result.Details.(Details); details.Op != c.wantOp {
				t.Fatalf("%s ran as %q, want %q", c.args, details.Op, c.wantOp)
			}
		})
	}
}

// TestAmbiguousCallKeepsItsMessage shortens nothing on the model-facing side: a
// call the arguments cannot resolve still reaches the model with the schema's
// own explanation, because that is the text it has to correct against.
func TestAmbiguousCallKeepsItsMessage(t *testing.T) {
	tool := tools.DefinitionAgentTool(Definition())
	var call ai.ToolCall
	if err := call.SetArgumentsJSON([]byte(`{"items":[]}`)); err != nil {
		t.Fatalf("SetArgumentsJSON: %v", err)
	}
	call.ID, call.Name = "call", ToolName
	outcome, err := agent.RunToolCall(t.Context(), call, agent.RunToolCallOptions{
		Tools:    []agent.AgentTool{tool},
		OnUpdate: func(agent.AgentToolResult) error { return nil },
	})
	if err != nil {
		t.Fatalf("RunToolCall: %v", err)
	}
	if !outcome.IsError {
		t.Fatal("an ambiguous payload must still fail")
	}
	message := outcome.Result.Text()
	if !strings.Contains(message, "op: missing") || !strings.Contains(message, "Received arguments:") {
		t.Fatalf("the model must receive the schema's own explanation, got:\n%s", message)
	}
}
