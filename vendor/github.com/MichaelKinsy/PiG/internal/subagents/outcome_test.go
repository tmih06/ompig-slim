package subagents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

// retrieval drives the model-facing tool path, which is where a parent loses
// the difference between a failed child and an intentionally empty one.
func retrieval(t *testing.T, records []Record, args string) string {
	t.Helper()
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Restore: func() []Record { return records }})
	t.Cleanup(m.Close)
	if err := m.Update(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	tool := Tool{Manager: m, ToolName: "get_subagent_result"}
	result, err := tool.Execute(context.Background(), "result", []byte(args), nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.Content[0].(ai.TextContent).Text
}

func TestRetrievalSeparatesEmptySuccessFromFailedAndCanceled(t *testing.T) {
	for _, test := range []struct {
		name    string
		record  Record
		want    []string
		exactly *string
	}{
		{name: "completed empty", record: Record{ID: "a", Status: "completed"}, exactly: new("")},
		{name: "completed text", record: Record{ID: "a", Status: "completed", Result: "auth.go"}, exactly: new("auth.go")},
		{name: "failed empty", record: Record{ID: "a", Status: "error", Error: "provider rejected the request"}, want: []string{"subagent_outcome: failed", "provider rejected the request", "partial_output: none"}},
		{name: "canceled empty", record: Record{ID: "a", Status: "stopped"}, want: []string{"subagent_outcome: canceled", "partial_output: none"}},
		{name: "exhausted empty", record: Record{ID: "a", Status: "exhausted"}, want: []string{"subagent_outcome: exhausted", "partial_output: none"}},
		{name: "canceled partial", record: Record{ID: "a", Status: "stopped", Result: "half an answer"}, want: []string{"subagent_outcome: canceled", "half an answer", "partial_output"}},
		{name: "failed partial", record: Record{ID: "a", Status: "error", Result: "half an answer", Error: "gate failed"}, want: []string{"subagent_outcome: failed", "half an answer", "gate failed"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			text := retrieval(t, []Record{test.record}, `{"agent_id":"a"}`)
			if test.exactly != nil {
				if text != *test.exactly {
					t.Fatalf("completed retrieval must return the exact result text, got %q", text)
				}
				return
			}
			for _, want := range test.want {
				if !strings.Contains(text, want) {
					t.Fatalf("retrieval lost %q from %q", want, text)
				}
			}
		})
	}
}

func TestRetrievalReportsRunningChild(t *testing.T) {
	release := make(chan struct{})
	m := testManager(t, func(context.Context, Invocation) (Child, error) {
		return &fakeChild{send: func(ctx context.Context, _ string) (string, error) {
			select {
			case <-release:
				return "done", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}}, nil
	})
	defer close(release)
	r, err := m.Spawn(context.Background(), Params{Prompt: "work", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return current.Status == "running" })
	tool := Tool{Manager: m, ToolName: "get_subagent_result"}
	result, err := tool.Execute(context.Background(), "result", []byte(`{"agent_id":"`+r.ID+`"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if text := result.Content[0].(ai.TextContent).Text; !strings.Contains(text, "subagent_outcome: running") {
		t.Fatalf("a running child must not read as finished: %q", text)
	}
}

func TestRetrievalMarksPartialOutputWithoutInventingIt(t *testing.T) {
	text := retrieval(t, []Record{{ID: "a", Status: "exhausted", Result: "two of five files"}}, `{"agent_id":"a"}`)
	if !strings.Contains(text, "incomplete") || !strings.Contains(text, "two of five files") {
		t.Fatalf("partial text is not labelled as partial: %q", text)
	}
	if text := retrieval(t, []Record{{ID: "a", Status: "exhausted"}}, `{"agent_id":"a"}`); strings.Contains(text, "incomplete") {
		t.Fatalf("retrieval fabricated partial text: %q", text)
	}
}

func TestOutcomeDerivationCoversEveryStatus(t *testing.T) {
	for status, want := range map[string]Outcome{
		"queued":    OutcomeRunning,
		"running":   OutcomeRunning,
		"scheduled": OutcomeRunning,
		"completed": OutcomeCompleted,
		"steered":   OutcomeCompleted,
		"error":     OutcomeFailed,
		"stopped":   OutcomeCanceled,
		"aborted":   OutcomeCanceled,
		"exhausted": OutcomeExhausted,
	} {
		if got := (Record{Status: status}).Outcome(); got != want {
			t.Fatalf("status %q reported outcome %q, want %q", status, got, want)
		}
	}
}

func TestCancelledWaitStaysParentCancellation(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	child := &fakeChild{send: func(ctx context.Context, p string) (string, error) {
		close(started)
		select {
		case <-release:
			return "finished", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}}
	var creations atomic.Int32
	m := testManager(t, func(context.Context, Invocation) (Child, error) { creations.Add(1); return child, nil })
	defer close(release)
	r, err := m.Spawn(context.Background(), Params{Prompt: "work", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	tool := Tool{Manager: m, ToolName: "get_subagent_result"}
	// Cancel while the child is genuinely running, which is the case a parent
	// timeout actually produces.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err = tool.Execute(ctx, "result", []byte(`{"agent_id":"`+r.ID+`","wait":true}`), nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a cancelled wait reported %v, want the parent's own cancellation", err)
	}
	if current, _ := m.Find(r.ID); !current.Active() || current.Result != "" {
		t.Fatalf("cancelling the wait changed the child: %+v", current)
	}
	// A cancelled wait is the parent's timeout, not new work for the child.
	time.Sleep(200 * time.Millisecond)
	if creations.Load() != 1 {
		t.Fatalf("cancelling the wait resubmitted work: %d creations", creations.Load())
	}
}

func TestHardTurnBudgetIsNotParentCancellation(t *testing.T) {
	child := &fakeChild{}
	child.send = func(ctx context.Context, _ string) (string, error) {
		child.mu.Lock()
		listener := child.listener
		child.mu.Unlock()
		for range 6 {
			listener(agent.TurnStartEvent{})
		}
		<-ctx.Done()
		return "last usable answer", ctx.Err()
	}
	m := testManager(t, func(context.Context, Invocation) (Child, error) { return child, nil })
	maxTurns := 1
	r, err := m.Spawn(context.Background(), Params{Prompt: "work", MaxTurns: &maxTurns}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	current, _ := m.Find(r.ID)
	if current.Outcome() != OutcomeExhausted {
		t.Fatalf("hard turn budget reported %q (%s), want exhausted", current.Outcome(), current.Status)
	}
	if current.Result != "last usable answer" {
		t.Fatalf("budget exhaustion dropped the partial answer: %q", current.Result)
	}
	text := retrieval(t, []Record{current}, `{"agent_id":"`+r.ID+`","wait":true}`)
	if !strings.Contains(text, "subagent_outcome: exhausted") {
		t.Fatalf("retrieval lost budget exhaustion: %q", text)
	}
}

func TestGracePeriodCompletionStaysSuccessful(t *testing.T) {
	child := &fakeChild{}
	child.send = func(context.Context, string) (string, error) {
		child.mu.Lock()
		listener := child.listener
		child.mu.Unlock()
		// One turn trips the soft limit; the next two are grace turns, and the
		// child answers well before MaxTurns+GraceTurns is reached.
		for range 3 {
			listener(agent.TurnStartEvent{})
		}
		return "wrapped up in time", nil
	}
	m := testManager(t, func(context.Context, Invocation) (Child, error) { return child, nil })
	maxTurns := 1
	r, err := m.Spawn(context.Background(), Params{Prompt: "work", MaxTurns: &maxTurns}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	current, _ := m.Find(r.ID)
	if current.Outcome() != OutcomeCompleted {
		t.Fatalf("grace-period completion reported %q (%s), want completed", current.Outcome(), current.Status)
	}
	if current.Partial() {
		t.Fatal("a completed child was marked partial")
	}
	if text := retrieval(t, []Record{current}, `{"agent_id":"`+r.ID+`"}`); text != "wrapped up in time" {
		t.Fatalf("completed retrieval gained status prose: %q", text)
	}
}

func TestParentCancellationIsNotBudgetExhaustion(t *testing.T) {
	started := make(chan struct{})
	child := &fakeChild{send: func(ctx context.Context, _ string) (string, error) {
		close(started)
		<-ctx.Done()
		return "partial work", ctx.Err()
	}}
	m := testManager(t, func(context.Context, Invocation) (Child, error) { return child, nil })
	maxTurns := 1
	r, err := m.Spawn(context.Background(), Params{Prompt: "work", MaxTurns: &maxTurns}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err = m.Stop(r.ID, ""); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	current, _ := m.Find(r.ID)
	if current.Outcome() != OutcomeCanceled || current.Result != "partial work" {
		t.Fatalf("parent cancellation: outcome %q result %q", current.Outcome(), current.Result)
	}
}

func TestOutcomeSurvivesReload(t *testing.T) {
	// Reload goes through the persisted record and the manager's restore pass,
	// which must not turn a terminal failure into apparent success.
	for _, test := range []struct {
		status  string
		want    Outcome
		wantErr string
	}{
		{"error", OutcomeFailed, ""},
		{"stopped", OutcomeCanceled, ""},
		{"exhausted", OutcomeExhausted, ""},
		{"completed", OutcomeCompleted, ""},
		// A crash mid-run is a cancellation, never an empty success.
		{"running", OutcomeCanceled, "Session ended before the agent completed"},
	} {
		b, err := json.Marshal(Record{ID: "a", Status: test.status, Result: "kept"})
		if err != nil {
			t.Fatal(err)
		}
		var saved Record
		if err = json.Unmarshal(b, &saved); err != nil {
			t.Fatal(err)
		}
		m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Restore: func() []Record { return []Record{saved} }})
		restored, ok := m.Find("a")
		m.Close()
		if !ok {
			t.Fatalf("restore dropped the %s record", test.status)
		}
		if restored.Outcome() != test.want || restored.Result != "kept" || test.wantErr != restored.Error {
			t.Fatalf("reload of %q gave outcome %q error %q", test.status, restored.Outcome(), restored.Error)
		}
	}
}

// launchAgent drives the model-facing Agent call and returns the child it
// started, which is always a background child.
func launchAgent(t *testing.T, m *Manager) Record {
	t.Helper()
	tool := Tool{Manager: m, ToolName: "Agent"}
	result, err := tool.Execute(context.Background(), "call", []byte(`{"prompt":"inspect","description":"Inspect code","role":"@task","tool":["read"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := result.Details.(Record)
	if !ok || !r.Background {
		t.Fatalf("Agent did not launch a background child: %#v", result.Details)
	}
	return r
}

func retrieveAgent(t *testing.T, m *Manager, id string) agent.AgentToolResult {
	t.Helper()
	tool := Tool{Manager: m, ToolName: "get_subagent_result"}
	result, err := tool.Execute(context.Background(), "result", []byte(`{"agent_id":"`+id+`","wait":true}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestAgentCallThenRetrievalReportsFailedOutcome(t *testing.T) {
	m := testManager(t, func(context.Context, Invocation) (Child, error) {
		return &fakeChild{send: func(context.Context, string) (string, error) { return "", errors.New("provider rejected the request") }}, nil
	})
	r := launchAgent(t, m)
	result := retrieveAgent(t, m, r.ID)
	envelope, ok := result.Details.(OutcomeEnvelope)
	if !ok || envelope.Outcome != OutcomeFailed {
		t.Fatalf("retrieval lost the outcome: %#v", result.Details)
	}
	if !result.IsError || !strings.Contains(result.Content[0].(ai.TextContent).Text, "subagent_outcome: failed") {
		t.Fatalf("an empty failure reads as success: %q", result.Content)
	}
}

func TestAgentCallThenRetrievalKeepsExactCompletedText(t *testing.T) {
	m := testManager(t, func(context.Context, Invocation) (Child, error) {
		return &fakeChild{send: func(context.Context, string) (string, error) { return "auth.go", nil }}, nil
	})
	r := launchAgent(t, m)
	result := retrieveAgent(t, m, r.ID)
	if text := result.Content[0].(ai.TextContent).Text; text != "auth.go" {
		t.Fatalf("completed result gained status prose: %q", text)
	}
	if result.IsError {
		t.Fatal("a completed child reported an error result")
	}
}

func TestNotificationReportsOutcomeAndKeepsBoundedPreview(t *testing.T) {
	var notification string
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Restore: func() []Record {
		return []Record{{ID: "a", Status: "exhausted", Result: strings.Repeat("界", 1600), CompletedAt: 1}}
	}, Notify: func(text string) { notification = text }})
	defer m.Close()
	m.mu.Lock()
	m.runs[0].consumed = false
	m.notifications = []completion{{"a", 1}}
	m.mu.Unlock()
	m.flushNotifications()
	if !strings.Contains(notification, "<outcome>exhausted</outcome>") {
		t.Fatalf("notification lost the outcome: %q", notification)
	}
	if !strings.Contains(notification, "<partial>true</partial>") {
		t.Fatalf("notification lost the partial marker: %q", notification)
	}
	if !strings.Contains(notification, "use get_subagent_result") {
		t.Fatalf("bounded notification lost the retrieval hint: %q", notification)
	}
}
