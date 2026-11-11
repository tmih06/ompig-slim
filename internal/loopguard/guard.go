// Package loopguard ports oh-my-pi's consecutive tool-call detector and
// corrective steer. It observes completed turns, never blocks tool execution,
// and leaves the decision to change approach or yield to the model.
package loopguard

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
)

const (
	// threshold counts whole assistant turns, not individual parallel calls.
	threshold  = 5
	customType = "tool-call-loop-redirect"
)

// detection is the bounded diagnostic saved alongside the hidden steer.
// Arguments and results are summaries, not copies of potentially huge outputs.
type detection struct {
	ToolName         string `json:"toolName"`
	Count            int    `json:"count"`
	ArgumentsSummary string `json:"argumentsSummary"`
	ResultSummary    string `json:"resultSummary"`
}

// guard owns one run's consecutive-turn fingerprint. The extension serializes
// access; separate factory invocations must never share this state.
type guard struct {
	last  string
	count int
}

// Extension registers the primary-session guard without adding tools or UI.
// Each run starts fresh; history, session switches and reloads cannot spend a
// new run's threshold. The mutex also protects against lifecycle dispatch from
// another goroutine. Steers are boundary drafts, so persistence and model context
// are committed together and no asynchronous send can miss the next request.
func Extension(api extension.API) error {
	var mu sync.Mutex
	var state guard
	api.OnAgentStart(func(_ context.Context, _ extension.AgentStartEvent) error {
		mu.Lock()
		defer mu.Unlock()
		state = guard{}
		return nil
	})
	api.OnTurnEnd(func(_ context.Context, event extension.TurnEndEvent) (extension.TurnEndEventResult, error) {
		mu.Lock()
		defer mu.Unlock()
		if event.Outcome != extension.AgentActivityCompleted {
			state = guard{}
			return extension.TurnEndEventResult{}, nil
		}
		detected, err := state.record(event)
		if err != nil || detected == nil {
			return extension.TurnEndEventResult{}, err
		}
		// Preserve drafts proposed by earlier handlers and do not override their
		// continuation choice. Tool results already make the next turn runnable.
		entries := append([]extension.SessionBoundaryDraft(nil), event.Entries...)
		entries = append(entries, extension.SessionBoundaryDraft{
			Type:       extension.SessionBoundaryDraftCustomMessage,
			CustomType: customType, Content: detected.render(), Details: detected,
		})
		return extension.TurnEndEventResult{Entries: &entries}, nil
	})
	return nil
}

// record compares sorted whole-turn call sets, preserving duplicate calls but
// ignoring call IDs, ordering, result changes and harness intent fields. A
// no-call or all-wait turn resets the sequence. Unlike OMP, PiG has no useless
// result marker, so it cannot carry the counter across idle exempt waits.
// Invalid argument serialization resets the sequence and reports the error.
func (g *guard) record(event extension.TurnEndEvent) (*detection, error) {
	var calls []ai.ToolCall
	if event.Message.Assistant != nil {
		for _, block := range event.Message.Assistant.Content {
			if call, ok := block.(ai.ToolCall); ok {
				calls = append(calls, call)
			}
		}
	}
	var report *ai.ToolCall
	for i := range calls {
		if calls[i].Name != "wait" {
			report = &calls[i]
			break
		}
	}
	if report == nil {
		*g = guard{}
		return nil, nil
	}
	keys := make([]string, 0, len(calls))
	var reportArgs string
	for _, call := range calls {
		args, err := canonicalArguments(call)
		if err != nil {
			*g = guard{}
			return nil, err
		}
		key, _ := json.Marshal([]string{call.Name, args})
		keys = append(keys, string(key))
		if call.ID == report.ID {
			reportArgs = args
		}
	}
	sort.Strings(keys)
	encoded, _ := json.Marshal(keys)
	key := string(encoded)
	if key == g.last {
		g.count++
	} else {
		g.last, g.count = key, 1
	}
	if g.count < threshold {
		return nil, nil
	}
	var resultText []string
	for _, result := range event.ToolResults {
		if result.ToolCallID != report.ID {
			continue
		}
		for _, block := range result.Content {
			if text, ok := block.(ai.TextContent); ok {
				resultText = append(resultText, text.Text)
			}
		}
		break
	}
	return &detection{
		ToolName: report.Name, Count: g.count,
		ArgumentsSummary: summarize(reportArgs, 400),
		ResultSummary:    summarize(strings.Join(resultText, "\n"), 200),
	}, nil
}

// canonicalArguments round-trips through JSON so recorded argument key ordering
// cannot affect comparison. It copies rather than mutating the tool's inputs;
// encoding/json sorts object keys recursively while retaining array order.
func canonicalArguments(call ai.ToolCall) (string, error) {
	data, err := call.ArgumentsJSON()
	if err != nil {
		return "", err
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return "", err
	}
	stripIntent(value)
	data, err = json.Marshal(value)
	return string(data), err
}

// stripIntent removes only the current and legacy OMP harness metadata keys,
// including nested objects and arrays, matching the upstream detector.
func stripIntent(value any) {
	switch value := value.(type) {
	case map[string]any:
		delete(value, "i")
		delete(value, "__intent")
		for _, child := range value {
			stripIntent(child)
		}
	case []any:
		for _, child := range value {
			stripIntent(child)
		}
	}
}

// summarize collapses whitespace and truncates in Unicode characters, retaining
// valid UTF-8 in the persisted diagnostic. Limits exclude the trailing ellipsis.
func summarize(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}

// render mirrors OMP's corrective rather than terminating or restarting a run.
// Continued identical turns earn another steer, not a fresh threshold window.
func (d detection) render() string {
	result := d.ResultSummary
	if result == "" {
		result = "(no text result)"
	}
	return fmt.Sprintf("<system-interrupt reason=\"tool_call_loop_detected\">\nYou called `%s` %d consecutive times with identical arguments:\n`%s`\n\nLast result (truncated): `%s`\n\nNEVER call `%s` with those arguments again this turn. Use different arguments, choose another tool, or summarize findings and yield if complete.\n</system-interrupt>", d.ToolName, d.Count, d.ArgumentsSummary, result, d.ToolName)
}
