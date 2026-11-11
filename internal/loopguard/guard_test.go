package loopguard

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
)

// call decodes provider-shaped arguments, retaining the key order the model sent.
func call(t *testing.T, id, name, args string) ai.ToolCall {
	t.Helper()
	var result ai.ToolCall
	if err := json.Unmarshal([]byte(fmt.Sprintf(`{"type":"toolCall","id":%q,"name":%q,"arguments":%s}`, id, name, args)), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

// turn builds a successful assistant boundary; results may be supplied by a test.
func turn(calls ...ai.ToolCall) extension.TurnEndEvent {
	content := make([]ai.AssistantContentBlock, len(calls))
	for i, call := range calls {
		content[i] = call
	}
	return extension.TurnEndEvent{
		Type: "turn_end", BoundaryState: &extension.BoundaryState{Outcome: extension.AgentActivityCompleted},
		Message: agent.AgentMessage{Assistant: &agent.AssistantMessage{Role: "assistant", Content: content}},
	}
}

// TestConsecutiveTurns covers the threshold, reset paths, batch ordering and
// duplicate multiplicity. Different result text is deliberately not progress.
func TestConsecutiveTurns(t *testing.T) {
	a := call(t, "a", "read", `{"path":"a"}`)
	b := call(t, "b", "read", `{"path":"b"}`)
	wait := call(t, "w", "wait", `{}`)
	for _, test := range []struct {
		name   string
		turns  []extension.TurnEndEvent
		counts []int
	}{
		{"threshold and continued repetition", []extension.TurnEndEvent{turn(a), turn(a), turn(a), turn(a), turn(a), turn(a)}, []int{0, 0, 0, 0, 5, 6}},
		{"changed arguments", []extension.TurnEndEvent{turn(a), turn(a), turn(a), turn(a), turn(b), turn(a)}, []int{0, 0, 0, 0, 0, 0}},
		{"no calls", []extension.TurnEndEvent{turn(a), turn(a), turn(a), turn(a), turn(), turn(a)}, []int{0, 0, 0, 0, 0, 0}},
		{"exempt wait", []extension.TurnEndEvent{turn(a), turn(a), turn(a), turn(a), turn(wait), turn(a)}, []int{0, 0, 0, 0, 0, 0}},
		{"parallel ordering", []extension.TurnEndEvent{turn(a, b), turn(b, a), turn(a, b), turn(b, a), turn(a, b)}, []int{0, 0, 0, 0, 5}},
		{"duplicate multiplicity", []extension.TurnEndEvent{turn(a), turn(a), turn(a), turn(a), turn(a, a)}, []int{0, 0, 0, 0, 0}},
		{"mixed exempt batch", []extension.TurnEndEvent{turn(wait, a), turn(a, wait), turn(wait, a), turn(a, wait), turn(wait, a)}, []int{0, 0, 0, 0, 5}},
		{"alternating batches", []extension.TurnEndEvent{turn(a), turn(b), turn(a), turn(b), turn(a), turn(b)}, []int{0, 0, 0, 0, 0, 0}},
		{"different tool", []extension.TurnEndEvent{turn(a), turn(a), turn(a), turn(a), turn(call(t, "c", "find", `{"path":"a"}`))}, []int{0, 0, 0, 0, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var g guard
			for i, event := range test.turns {
				detected, err := g.record(event)
				if err != nil {
					t.Fatal(err)
				}
				got := 0
				if detected != nil {
					got = detected.Count
					if detected.ToolName != "read" {
						t.Fatal("reported exempt tool", detected)
					}
				}
				if got != test.counts[i] {
					t.Fatalf("turn %d: count %d, want %d", i, got, test.counts[i])
				}
			}
		})
	}
}

// TestCanonicalArguments verifies recursive sorting and intent stripping without
// mutating original inputs or conflating meaningful array ordering.
func TestCanonicalArguments(t *testing.T) {
	a := call(t, "a", "read", `{"z":[{"b":2,"a":1,"i":"first"}],"a":true,"__intent":"x"}`)
	b := call(t, "b", "read", `{"i":"other","a":true,"z":[{"__intent":"y","a":1,"b":2}]}`)
	before, _ := a.ArgumentsJSON()
	x, err := canonicalArguments(a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := canonicalArguments(b)
	if err != nil || x != y {
		t.Fatalf("canonical arguments differ: %s / %s (%v)", x, y, err)
	}
	after, _ := a.ArgumentsJSON()
	if string(before) != string(after) {
		t.Fatal("modified original arguments")
	}
	var g guard
	for i := 1; i <= threshold; i++ {
		c := a
		if i%2 == 0 {
			c = b
		}
		detected, err := g.record(turn(c))
		if err != nil || (detected != nil) != (i == threshold) {
			t.Fatalf("turn %d: %v, %v", i, detected, err)
		}
	}
	first, _ := canonicalArguments(call(t, "a", "read", `{"items":[1,2]}`))
	second, _ := canonicalArguments(call(t, "a", "read", `{"items":[2,1]}`))
	if first == second {
		t.Fatal("array ordering lost")
	}
}

// TestDiagnosticBounds verifies result-ID matching, Unicode-safe truncation,
// whitespace folding and the no-text fallback used for image-only results.
func TestDiagnosticBounds(t *testing.T) {
	c := call(t, "a", "read", fmt.Sprintf(`{"path":%q}`, strings.Repeat("界", 500)))
	var g guard
	var detected *detection
	for i := 0; i < threshold; i++ {
		event := turn(c)
		event.ToolResults = []agent.ToolResultMessage{
			{ToolCallID: "unrelated", Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "wrong"}}},
			{ToolCallID: "a", IsError: true, Content: []ai.ToolResultMessageContent{ai.TextContent{Text: fmt.Sprintf(" attempt %d\n\t%s", i, strings.Repeat("界", 250))}}},
		}
		var err error
		detected, err = g.record(event)
		if err != nil {
			t.Fatal(err)
		}
	}
	if detected == nil || utf8.RuneCountInString(detected.ArgumentsSummary) != 401 || utf8.RuneCountInString(detected.ResultSummary) != 201 {
		t.Fatalf("unbounded diagnostic: %+v", detected)
	}
	if !utf8.ValidString(detected.render()) || !strings.HasPrefix(detected.ResultSummary, "attempt 4 ") {
		t.Fatal(detected)
	}
	detected.ResultSummary = ""
	if !strings.Contains(detected.render(), "(no text result)") {
		t.Fatal("missing fallback")
	}
}

// captureAPI captures only the two handlers this extension registers, letting
// lifecycle tests exercise the factory without inventing asynchronous delivery.
type captureAPI struct {
	extension.API
	start func(context.Context, extension.AgentStartEvent) error
	end   func(context.Context, extension.TurnEndEvent) (extension.TurnEndEventResult, error)
}

// OnAgentStart retains the run-reset handler; the test owns dispatch ordering.
func (a *captureAPI) OnAgentStart(handler extension.ExtensionHandlerNoResult[extension.AgentStartEvent]) func() {
	a.start = handler
	return func() {}
}

// OnTurnEnd retains the awaited boundary handler for synchronous dispatch.
func (a *captureAPI) OnTurnEnd(handler extension.ExtensionHandler[extension.TurnEndEvent, extension.TurnEndEventResult]) func() {
	a.end = handler
	return func() {}
}

// TestBoundaryLifecycle protects previous drafts and continuation decisions,
// resetting on new runs, aborted/error turns and independent session factories.
func TestBoundaryLifecycle(t *testing.T) {
	ctx := context.Background()
	api := &captureAPI{}
	if err := Extension(api); err != nil {
		t.Fatal(err)
	}
	other := &captureAPI{}
	if err := Extension(other); err != nil {
		t.Fatal(err)
	}
	event := turn(call(t, "a", "read", `{"path":"a"}`))
	prior := extension.SessionBoundaryDraft{Type: extension.SessionBoundaryDraftCustom, CustomType: "earlier", Data: "preserve"}
	event.Entries = []extension.SessionBoundaryDraft{prior}
	for i := 1; i <= threshold+1; i++ {
		result, err := api.end(ctx, event)
		if err != nil || result.Continue != nil {
			t.Fatal(result, err)
		}
		if i < threshold {
			if result.Entries != nil {
				t.Fatal("early steer")
			}
			continue
		}
		entries := *result.Entries
		if len(entries) != 2 || !reflect.DeepEqual(entries[0], prior) || entries[1].Display || entries[1].CustomType != customType {
			t.Fatal(entries)
		}
	}
	if len(event.Entries) != 1 {
		t.Fatal("mutated previous proposal")
	}
	result, err := other.end(ctx, event)
	if err != nil || result.Entries != nil {
		t.Fatal("shared state across factories", result, err)
	}
	for _, outcome := range []extension.AgentActivityOutcome{extension.AgentActivityAborted, extension.AgentActivityError} {
		event.Outcome = outcome
		if result, err = api.end(ctx, event); err != nil || result.Entries != nil {
			t.Fatal("steered unsuccessful turn", result, err)
		}
		event.Outcome = extension.AgentActivityCompleted
		for i := 0; i < threshold-1; i++ {
			if result, err = api.end(ctx, event); err != nil || result.Entries != nil {
				t.Fatal("unsuccessful turn did not reset", result, err)
			}
		}
	}
	if err := api.start(ctx, extension.AgentStartEvent{}); err != nil {
		t.Fatal(err)
	}
	if result, err = api.end(ctx, event); err != nil || result.Entries != nil {
		t.Fatal("new run did not reset", result, err)
	}
}
