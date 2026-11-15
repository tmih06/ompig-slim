package loopguard

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
)

const unmatchedError = "Could not find the exact text in a. The old text must match exactly including all whitespace and newlines."
const ambiguousError = "Found 2 occurrences of the text in a. The text must be unique. Please provide more context to make it unique."

func resultTurn(c ai.ToolCall, isError bool, text string) extension.TurnEndEvent {
	event := turn(c)
	event.ToolResults = []agent.ToolResultMessage{{ToolCallID: c.ID, ToolName: c.Name, IsError: isError, Content: []ai.ToolResultMessageContent{ai.TextContent{Text: text}}}}
	return event
}

func TestFailureClasses(t *testing.T) {
	for _, test := range []struct{ tool, text, class string }{
		{"edit", unmatchedError, "edit-unmatched"},
		{"edit", "Could not find edits[1] in a. The oldText must match exactly including all whitespace and newlines.", "edit-unmatched"},
		{"edit", ambiguousError, "edit-ambiguous"},
		{"edit", "Found 3 occurrences of edits[0] in a. Each oldText must be unique. Please provide more context to make it unique.", "edit-ambiguous"},
		{"grep", "regex parse error:\n    [\n    ^\nerror: unclosed character class", "grep-invalid-regex"},
		{"grep", "rg: regex parse error:\nerror: unclosed group", "grep-invalid-regex"},
		{"grep", "No matches found", ""},
		{"grep", "Path not found: a", ""},
		{"grep", "Failed to run ripgrep: connection reset", ""},
		{"grep", "Operation aborted", ""},
		{"edit", "Operation aborted", ""},
		{"edit", "Could not find file a", ""},
		{"bash", unmatchedError, ""},
		{"read", "regex parse error:", ""},
		{"bash", "test failed", ""},
	} {
		t.Run(test.tool+"/"+test.text, func(t *testing.T) {
			if got := deterministicClass(test.tool, test.text); got != test.class {
				t.Fatalf("class %q, want %q", got, test.class)
			}
		})
	}
}

func TestFailureStreaks(t *testing.T) {
	edit := func(path, text string) extension.TurnEndEvent {
		return resultTurn(call(t, "edit", "edit", fmt.Sprintf(`{"path":%q,"edits":[{"oldText":%q,"newText":"x"}]}`, path, text)), true, unmatchedError)
	}
	a, b, c := edit("a", "first"), edit("./a", "second"), edit("a", "third")
	read := resultTurn(call(t, "read", "read", `{"path":"a"}`), false, "fresh content")
	otherRead := resultTurn(call(t, "read", "read", `{"path":"b"}`), false, "unrelated")
	ambiguous := resultTurn(call(t, "edit", "edit", `{"path":"a","oldText":"x"}`), true, ambiguousError)
	batch := turn(a.Message.Assistant.Content[0].(ai.ToolCall), otherRead.Message.Assistant.Content[0].(ai.ToolCall))
	batch.ToolResults = append(append([]agent.ToolResultMessage{}, otherRead.ToolResults...), a.ToolResults...)
	successBatch := batch
	successBatch.Message = turn(a.Message.Assistant.Content[0].(ai.ToolCall), read.Message.Assistant.Content[0].(ai.ToolCall)).Message
	successBatch.ToolResults = append(append([]agent.ToolResultMessage{}, a.ToolResults...), read.ToolResults...)
	mixed := turn(a.Message.Assistant.Content[0].(ai.ToolCall), call(t, "other", "edit", `{"path":"b"}`))
	mixed.ToolResults = append(append([]agent.ToolResultMessage{}, a.ToolResults...), agent.ToolResultMessage{ToolCallID: "other", IsError: true, Content: []ai.ToolResultMessageContent{ai.TextContent{Text: unmatchedError}}})
	orphan := a
	orphan.ToolResults = []agent.ToolResultMessage{{ToolCallID: "unrelated", IsError: true, Content: []ai.ToolResultMessageContent{ai.TextContent{Text: unmatchedError}}}}
	for _, test := range []struct {
		name   string
		turns  []extension.TurnEndEvent
		steers []int
	}{
		{"varied anchors and normalized target", []extension.TurnEndEvent{a, b, c, a}, []int{3, 4}},
		{"read resets", []extension.TurnEndEvent{a, b, read, c, a, b}, []int{6}},
		{"unrelated success preserves", []extension.TurnEndEvent{a, b, otherRead, c}, []int{4}},
		{"different target resets", []extension.TurnEndEvent{a, b, edit("b", "other"), c, a, b}, []int{6}},
		{"different class resets", []extension.TurnEndEvent{a, b, ambiguous, c, a, b}, []int{6}},
		{"unrelated batch success", []extension.TurnEndEvent{batch, b, c}, []int{3}},
		{"same target batch success", []extension.TurnEndEvent{a, b, successBatch, c, a, b}, []int{6}},
		{"mixed targets clear batch", []extension.TurnEndEvent{a, b, mixed, c, a, b}, []int{6}},
		{"no calls", []extension.TurnEndEvent{a, b, turn(), c, a, b}, []int{6}},
		{"missing identity", []extension.TurnEndEvent{a, b, orphan, c, a, b}, []int{6}},
		{"unknown error", []extension.TurnEndEvent{a, b, resultTurn(call(t, "x", "bash", `{}`), true, "network failure"), c, a, b}, []int{6}},
		{"successful error text", []extension.TurnEndEvent{a, b, resultTurn(call(t, "x", "read", `{"path":"a"}`), false, unmatchedError), c, a, b}, []int{6}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var g guard
			for i, event := range test.turns {
				d, err := g.record(event)
				want := false
				for _, index := range test.steers {
					want = want || index == i+1
				}
				got := d != nil && d.FailureClass != ""
				if err != nil || got != want {
					t.Fatalf("turn %d: detection %+v, error %v, want steer %v", i+1, d, err, want)
				}
			}
		})
	}
}

func TestNestedFailureIdentity(t *testing.T) {
	for _, test := range []struct {
		name     string
		complete bool
		calls    []ai.NestedToolCallRecord
		want     bool
	}{
		{"reliable failed edit and unrelated success", true, []ai.NestedToolCallRecord{
			{Name: "read", Status: ai.NestedToolCallOK, Arguments: ai.JsonObject{"path": "b"}},
			{Name: "edit", Status: ai.NestedToolCallError, Arguments: ai.JsonObject{"path": "a"}, Error: unmatchedError},
		}, true},
		{"same target success wins", true, []ai.NestedToolCallRecord{
			{Name: "read", Status: ai.NestedToolCallOK, Arguments: ai.JsonObject{"path": "a"}},
			{Name: "edit", Status: ai.NestedToolCallError, Arguments: ai.JsonObject{"path": "a"}, Error: unmatchedError},
		}, false},
		{"incomplete records", false, []ai.NestedToolCallRecord{{Name: "edit", Status: ai.NestedToolCallError, Arguments: ai.JsonObject{"path": "a"}, Error: unmatchedError}}, false},
		{"omitted arguments", true, []ai.NestedToolCallRecord{{Name: "edit", Status: ai.NestedToolCallError, Error: unmatchedError}}, false},
		{"unfinished call", true, []ai.NestedToolCallRecord{{Name: "edit", Status: ai.NestedToolCallUnfinished, Arguments: ai.JsonObject{"path": "a"}, Error: unmatchedError}}, false},
		{"truncated error", true, []ai.NestedToolCallRecord{{Name: "edit", Status: ai.NestedToolCallError, Arguments: ai.JsonObject{"path": "a"}, Error: "Could not find the exact text in a..."}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var g guard
			for i := 0; i < 3; i++ {
				event := resultTurn(call(t, "outer", "codemode", fmt.Sprintf(`{"code":"attempt %d"}`, i)), false, unmatchedError)
				event.ToolResults[0].NestedCalls = &ai.NestedToolCalls{Complete: test.complete, Calls: test.calls}
				d, err := g.record(event)
				if err != nil || (d != nil) != (test.want && i == 2) {
					t.Fatalf("turn %d: %+v, %v", i+1, d, err)
				}
				if d != nil && d.ToolName != "edit" {
					t.Fatal("blamed unrelated call", d)
				}
			}
		})
	}
}

func TestFailureBatchCountsOnce(t *testing.T) {
	var g guard
	for i := 0; i < 3; i++ {
		var calls []ai.ToolCall
		var results []agent.ToolResultMessage
		for j := 0; j < 3; j++ {
			c := call(t, fmt.Sprint(j), "edit", fmt.Sprintf(`{"path":"a","oldText":"attempt %d/%d"}`, i, j))
			calls = append(calls, c)
			results = append(results, resultTurn(c, true, unmatchedError).ToolResults...)
		}
		event := turn(calls...)
		event.ToolResults = results
		d, err := g.record(event)
		if err != nil || (d != nil) != (i == 2) {
			t.Fatalf("turn %d: %+v, %v", i+1, d, err)
		}
	}
}

func TestFailureLifecycleAndBounds(t *testing.T) {
	ctx := context.Background()
	api, other := &captureAPI{}, &captureAPI{}
	if err := Extension(api); err != nil {
		t.Fatal(err)
	}
	if err := Extension(other); err != nil {
		t.Fatal(err)
	}
	event := resultTurn(call(t, "edit", "edit", fmt.Sprintf(`{"path":%q}`, strings.Repeat("界", 500))), true, unmatchedError)
	for _, reset := range []string{"new run", "aborted", "error"} {
		for i := 0; i < 2; i++ {
			if result, err := api.end(ctx, event); err != nil || result.Entries != nil {
				t.Fatal("early steer", result, err)
			}
		}
		if reset == "new run" {
			if err := api.start(ctx, extension.AgentStartEvent{}); err != nil {
				t.Fatal(err)
			}
		} else {
			failed := event
			failed.BoundaryState = &extension.BoundaryState{Outcome: extension.AgentActivityAborted}
			if reset == "error" {
				failed.Outcome = extension.AgentActivityError
			}
			if result, err := api.end(ctx, failed); err != nil || result.Entries != nil {
				t.Fatal(result, err)
			}
		}
		for i := 0; i < 3; i++ {
			result, err := api.end(ctx, event)
			if err != nil || (result.Entries != nil) != (i == 2) {
				t.Fatalf("%s turn %d: %+v, %v", reset, i+1, result, err)
			}
			if i == 2 {
				d := (*result.Entries)[0].Details.(*detection)
				if utf8.RuneCountInString(d.TargetSummary) != 201 || d.ArgumentsSummary != "" || d.ResultSummary != "" || !utf8.ValidString(d.render()) {
					t.Fatal("unbounded diagnostic", d)
				}
			}
		}
		if result, err := other.end(ctx, event); err != nil || result.Entries != nil {
			t.Fatal("shared state", result, err)
		}
		if err := other.start(ctx, extension.AgentStartEvent{}); err != nil {
			t.Fatal(err)
		}
		if err := api.start(ctx, extension.AgentStartEvent{}); err != nil {
			t.Fatal(err)
		}
	}
}
