package subagents

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

type resultRequestNotify struct {
	mu    sync.Mutex
	notes []string
}

func (n *resultRequestNotify) send(text string) {
	n.mu.Lock()
	n.notes = append(n.notes, text)
	n.mu.Unlock()
}
func (n *resultRequestNotify) all() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.notes...)
}

func resultRequestManager(t *testing.T, notes *resultRequestNotify, release <-chan struct{}, answer func(string) (string, error)) *Manager {
	t.Helper()
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), SessionID: "result-request", Notify: notes.send, ParentRunning: func() bool { return true },
		Create: func(context.Context, Invocation) (Child, error) {
			return &fakeChild{send: func(ctx context.Context, p string) (string, error) {
				select {
				case <-release:
					return answer(p)
				case <-ctx.Done():
					return "", ctx.Err()
				}
			}}, nil
		}})
	t.Cleanup(m.Close)
	if err := m.Update(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	return m
}

func resultTool(t *testing.T, m *Manager, args string) agent.AgentToolResult {
	t.Helper()
	tool := Tool{Manager: m, ToolName: "get_subagent_result"}
	result, err := tool.Execute(context.Background(), "result", []byte(args), nil)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func resultToolText(result agent.AgentToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	text, _ := result.Content[0].(ai.TextContent)
	return text.Text
}

func notificationResult(t *testing.T, note string) (id, status, outcome, result string, requested bool) {
	t.Helper()
	var task struct {
		ID        string `xml:"task-id"`
		Status    string `xml:"status"`
		Outcome   string `xml:"outcome"`
		Result    string `xml:"result"`
		Requested string `xml:"requested"`
		Text      string `xml:",chardata"`
	}
	if err := xml.Unmarshal([]byte(note), &task); err != nil {
		t.Fatalf("notification is not a task envelope: %v\n%s", err, note)
	}
	result = task.Result
	if result == "" {
		result = strings.TrimSpace(task.Text)
	}
	return task.ID, task.Status, task.Outcome, result, task.Requested != ""
}

// A root read of a running child returns a pending receipt at once, and the
// exact full result later arrives through the completion notification, not a
// bounded preview.
func TestResultRequestDeliversExactFullResultOnce(t *testing.T) {
	release := make(chan struct{})
	full := "RESULT_HEAD " + strings.Repeat("Unicode λ <tag> & 'quoted' exact text ", 80) + "RESULT_TAIL"
	if len(full) < maxToolResultPreview*2 {
		t.Fatal("fixture must exceed the preview bound")
	}
	notes := &resultRequestNotify{}
	m := resultRequestManager(t, notes, release, func(string) (string, error) { return full, nil })
	r, err := m.Spawn(context.Background(), Params{Prompt: "work", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return current.Status == "running" })

	first := resultTool(t, m, fmt.Sprintf(`{"agent_id":%q,"wait":true}`, r.ID))
	text := resultToolText(first)
	if !strings.Contains(text, "pending") || !strings.Contains(text, "running") {
		t.Fatalf("first read must be an immediate pending receipt: %q", text)
	}
	if envelope, ok := first.Details.(OutcomeEnvelope); !ok || envelope.Outcome != OutcomeRunning {
		t.Fatalf("receipt lost the running outcome: %#v", first.Details)
	}
	if first.IsError {
		t.Fatal("a pending child is not a failure")
	}
	close(release)
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	m.flushNotifications()
	notesAll := notes.all()
	if len(notesAll) != 1 {
		t.Fatalf("expected exactly one delivery, got %d: %v", len(notesAll), notesAll)
	}
	id, status, outcome, body, requested := notificationResult(t, notesAll[0])
	if id != r.ID || status != "completed" || outcome != string(OutcomeCompleted) || !requested {
		t.Fatalf("delivery envelope: id=%s status=%s outcome=%s requested=%v", id, status, outcome, requested)
	}
	if body != full {
		t.Fatalf("delivery did not carry the exact result: %d bytes vs %d", len(body), len(full))
	}
	m.flushNotifications()
	if got := len(notes.all()); got != 1 {
		t.Fatalf("the requested result delivered again: %d notifications", got)
	}
}

// A second read while the request is outstanding reports the existing request
// instead of scheduling another delivery.
func TestResultRequestDuplicateReadReportsAlreadyPending(t *testing.T) {
	release := make(chan struct{})
	notes := &resultRequestNotify{}
	m := resultRequestManager(t, notes, release, func(string) (string, error) { return "done", nil })
	r, err := m.Spawn(context.Background(), Params{Prompt: "work", Name: "reader", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return current.Status == "running" })
	first := resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))
	if !strings.Contains(resultToolText(first), "pending") {
		t.Fatalf("first read did not claim the result: %q", resultToolText(first))
	}
	// A duplicate read by canonical ID and one by handle share the claim.
	for _, ref := range []string{r.ID, "reader", "@reader"} {
		again := resultTool(t, m, fmt.Sprintf(`{"agent_id":%q,"wait":false}`, ref))
		text := resultToolText(again)
		if !strings.Contains(text, "already") || !strings.Contains(text, "pending") {
			t.Fatalf("duplicate read via %q did not report the pending request: %q", ref, text)
		}
	}
	close(release)
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	m.flushNotifications()
	if got := len(notes.all()); got != 1 {
		t.Fatalf("duplicate reads scheduled duplicate deliveries: %d", got)
	}
}

// Every incomplete outcome reaches the parent through the same requested
// delivery, carrying the retrieval semantics rather than a bare status.
func TestResultRequestDeliversIncompleteOutcomes(t *testing.T) {
	for _, test := range []struct {
		name    string
		answer  func(string) (string, error)
		status  string
		outcome Outcome
		want    []string
		exact   *string
	}{
		{name: "failed", answer: func(string) (string, error) { return "", errors.New("provider rejected") }, status: "error", outcome: OutcomeFailed,
			want: []string{"subagent_outcome: failed", "provider rejected", "partial_output: none"}},
		{name: "empty completed", answer: func(string) (string, error) { return "", nil }, status: "completed", outcome: OutcomeCompleted,
			exact: new("")},
	} {
		t.Run(test.name, func(t *testing.T) {
			release := make(chan struct{})
			notes := &resultRequestNotify{}
			m := resultRequestManager(t, notes, release, test.answer)
			r, err := m.Spawn(context.Background(), Params{Prompt: "work", Background: new(true)}, "", "")
			if err != nil {
				t.Fatal(err)
			}
			await(t, func() bool { current, _ := m.Find(r.ID); return current.Status == "running" })
			resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))
			close(release)
			await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
			m.flushNotifications()
			notesAll := notes.all()
			if len(notesAll) != 1 {
				t.Fatalf("deliveries: %d", len(notesAll))
			}
			_, status, outcome, body, requested := notificationResult(t, notesAll[0])
			if status != test.status || outcome != string(test.outcome) || !requested {
				t.Fatalf("envelope: %s %s requested=%v", status, outcome, requested)
			}
			if test.exact != nil {
				if body != *test.exact {
					t.Fatalf("an intentionally empty result arrived as %q", body)
				}
				return
			}
			for _, want := range test.want {
				if !strings.Contains(body, want) {
					t.Fatalf("delivery lost %q: %q", want, body)
				}
			}
		})
	}
}

// Reading the terminal record directly consumes the pending delivery, so the
// parent is not told twice about a result it already has.
func TestResultRequestTerminalReadSuppressesDelivery(t *testing.T) {
	release := make(chan struct{})
	notes := &resultRequestNotify{}
	m := resultRequestManager(t, notes, release, func(string) (string, error) { return "exact", nil })
	r, err := m.Spawn(context.Background(), Params{Prompt: "work", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return current.Status == "running" })
	resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))
	close(release)
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	read := resultTool(t, m, fmt.Sprintf(`{"agent_id":%q,"wait":true}`, r.ID))
	if got := resultToolText(read); got != "exact" {
		t.Fatalf("terminal read changed the result: %q", got)
	}
	m.flushNotifications()
	if got := notes.all(); len(got) != 0 {
		t.Fatalf("a consumed result still delivered: %v", got)
	}
}

// A resume between completion and dispatch must not lose the demanded result:
// the queued completion owns a frozen snapshot of the generation that produced
// it, and a demand on the new generation is independent.
func TestResultRequestResumeKeepsOldGeneration(t *testing.T) {
	// Gate each generation on its own release so the second cannot finish
	// before the first generation's delivery is observed.
	var calls atomic.Int32
	gates := []chan struct{}{make(chan struct{}), make(chan struct{})}
	notes := &resultRequestNotify{}
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), SessionID: "resume-request", Notify: notes.send,
		Create: func(context.Context, Invocation) (Child, error) {
			return &fakeChild{send: func(ctx context.Context, p string) (string, error) {
				call := calls.Add(1)
				select {
				case <-gates[min(int(call)-1, 1)]:
					return fmt.Sprintf("generation-%d", call), nil
				case <-ctx.Done():
					return "", ctx.Err()
				}
			}}, nil
		}})
	t.Cleanup(m.Close)
	if err := m.Update(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	r, err := m.Spawn(context.Background(), Params{Prompt: "first", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return current.Status == "running" })
	resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))
	close(gates[0])
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	resumed, err := m.Spawn(context.Background(), Params{Prompt: "second", Resume: r.ID, Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ID != r.ID {
		t.Fatalf("resume changed the agent id: %s", resumed.ID)
	}
	m.flushNotifications()
	notesAll := notes.all()
	if len(notesAll) != 1 {
		t.Fatalf("old-generation delivery lost to the resume: %v", notesAll)
	}
	_, _, _, body, requested := notificationResult(t, notesAll[0])
	if !requested || body != "generation-1" {
		t.Fatalf("old generation delivered the wrong payload: %q", body)
	}
	second := resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))
	if !strings.Contains(resultToolText(second), "pending") || strings.Contains(resultToolText(second), "already") {
		t.Fatalf("new generation did not get an independent claim: %q", resultToolText(second))
	}
	close(gates[1])
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	m.flushNotifications()
	notesAll = notes.all()
	if len(notesAll) != 2 {
		t.Fatalf("second generation delivered %d notifications", len(notesAll))
	}
	_, _, _, body, _ = notificationResult(t, notesAll[1])
	if body != "generation-2" {
		t.Fatalf("second generation delivered %q", body)
	}
}

// A claimed read bypasses sibling batching: the parent asked for this result,
// so it must not wait for a batch window.
func TestResultRequestBypassesSiblingBatching(t *testing.T) {
	releaseA := make(chan struct{})
	releaseB := make(chan struct{})
	notes := &resultRequestNotify{}
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), SessionID: "batch-request", Notify: notes.send,
		Create: func(_ context.Context, inv Invocation) (Child, error) {
			gate := releaseA
			if inv.Params.Prompt == "second" {
				gate = releaseB
			}
			return &fakeChild{send: func(ctx context.Context, p string) (string, error) {
				select {
				case <-gate:
					return p, nil
				case <-ctx.Done():
					return "", ctx.Err()
				}
			}}, nil
		}})
	t.Cleanup(m.Close)
	if err := m.Update(map[string]any{"enabled": true, "defaultJoinMode": "group"}); err != nil {
		t.Fatal(err)
	}
	a, err := m.Spawn(context.Background(), Params{Prompt: "first"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Spawn(context.Background(), Params{Prompt: "second"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool {
		ra, _ := m.Find(a.ID)
		rb, _ := m.Find(b.ID)
		return ra.Status == "running" && rb.Status == "running"
	})
	resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, a.ID))
	close(releaseA)
	await(t, func() bool { current, _ := m.Find(a.ID); return !current.Active() })
	m.flushNotifications()
	notesAll := notes.all()
	if len(notesAll) != 1 {
		t.Fatalf("demanded result batched behind the sibling: %v", notesAll)
	}
	_, _, _, body, requested := notificationResult(t, notesAll[0])
	if !requested || body != "first" {
		t.Fatalf("demanded delivery: %q", body)
	}
	close(releaseB)
	await(t, func() bool { current, _ := m.Find(b.ID); return !current.Active() })
	m.flushNotifications()
	if got := len(notes.all()); got != 2 {
		t.Fatalf("sibling delivery lost: %d", got)
	}
}

// A root read of a running workflow claims its result the same way, and the
// workflow's terminal ResultText is delivered once.
func TestResultRequestWorkflowDelivery(t *testing.T) {
	release := make(chan struct{})
	notes := &resultRequestNotify{}
	m := resultRequestManager(t, notes, release, func(string) (string, error) { return "FLOW_VALUE", nil })
	wf, err := m.StartWorkflow(context.Background(), WorkflowParams{Script: `export const meta={name:'flow',description:'Request proof'}; return await agent('work')`})
	if err != nil {
		t.Fatal(err)
	}
	result := resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, wf.ID))
	if !strings.Contains(resultToolText(result), "pending") {
		t.Fatalf("workflow read did not claim the result: %q", resultToolText(result))
	}
	again := resultTool(t, m, fmt.Sprintf(`{"agent_id":%q,"wait":true}`, wf.ID))
	if !strings.Contains(resultToolText(again), "already") {
		t.Fatalf("duplicate workflow read did not report the request: %q", resultToolText(again))
	}
	close(release)
	await(t, func() bool { current, _ := m.Workflow(wf.ID); return !current.Active() })
	m.flushNotifications()
	notesAll := notes.all()
	if len(notesAll) != 1 {
		t.Fatalf("workflow deliveries: %d", len(notesAll))
	}
	id, status, _, body, requested := notificationResult(t, notesAll[0])
	if id != wf.ID || status != "completed" || !requested || !strings.Contains(body, "FLOW_VALUE") {
		t.Fatalf("workflow delivery: %s %s %q requested=%v", id, status, body, requested)
	}
}

// A cancelled tool call leaves the claim in place: the demand belongs to the
// session, not the caller's context.
func TestResultRequestSurvivesCallerCancellation(t *testing.T) {
	release := make(chan struct{})
	notes := &resultRequestNotify{}
	m := resultRequestManager(t, notes, release, func(string) (string, error) { return "delivered", nil })
	r, err := m.Spawn(context.Background(), Params{Prompt: "work", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return current.Status == "running" })
	ctx, cancel := context.WithCancel(context.Background())
	tool := Tool{Manager: m, ToolName: "get_subagent_result"}
	result, err := tool.Execute(ctx, "result", []byte(fmt.Sprintf(`{"agent_id":%q}`, r.ID)), nil)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resultToolText(result), "pending") {
		t.Fatalf("cancelled caller lost its receipt: %q", resultToolText(result))
	}
	close(release)
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	m.flushNotifications()
	if got := len(notes.all()); got != 1 {
		t.Fatalf("caller cancellation cancelled the demand: %d deliveries", got)
	}
}

// A nested agent keeps the synchronous contract: wait:false observes without
// claiming and wait:true blocks for its own child only.
func TestResultRequestNestedKeepsSynchronousContract(t *testing.T) {
	parentRelease := make(chan struct{})
	nestedRelease := make(chan struct{})
	m := testManager(t, func(_ context.Context, inv Invocation) (Child, error) {
		gate := parentRelease
		if inv.Params.Prompt == "nested" {
			gate = nestedRelease
		}
		return &fakeChild{send: func(ctx context.Context, p string) (string, error) {
			select {
			case <-gate:
				return p, nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}}, nil
	})
	defer close(parentRelease)
	path := filepath.Join(m.host.CWD, ".pi", "agents")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "parent.md"), []byte("---\nname: parent\ndescription: Parent\nallowed_subagents: [agent]\n---\nDelegate."), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Update(map[string]any{}); err != nil {
		t.Fatal(err)
	}
	parent, err := m.Spawn(context.Background(), Params{Type: "parent", Prompt: "parent", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(parent.ID); return current.Status == "running" })
	child, err := m.Spawn(context.Background(), Params{Prompt: "nested"}, parent.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	tool := Tool{Manager: m, ToolName: "get_subagent_result", Owner: parent.ID}
	peek, err := tool.Execute(context.Background(), "peek", []byte(fmt.Sprintf(`{"agent_id":%q}`, child.ID)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if text := resultToolText(peek); !strings.Contains(text, "subagent_outcome: running") {
		t.Fatalf("nested wait:false stopped being observational: %q", text)
	}
	blocked := make(chan error, 1)
	go func() {
		result, err := tool.Execute(context.Background(), "wait", []byte(fmt.Sprintf(`{"agent_id":%q,"wait":true}`, child.ID)), nil)
		if err == nil {
			text := resultToolText(result)
			if !strings.Contains(text, "nested") {
				err = fmt.Errorf("nested wait returned %q", text)
			}
		}
		blocked <- err
	}()
	select {
	case err := <-blocked:
		t.Fatalf("nested wait:true returned early: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(nestedRelease)
	if err := <-blocked; err != nil {
		t.Fatal(err)
	}
}

func TestResultRequestTerminalReadKeepsExactText(t *testing.T) {
	release := make(chan struct{})
	notes := &resultRequestNotify{}
	m := resultRequestManager(t, notes, release, func(string) (string, error) { return "kept", nil })
	r, err := m.Spawn(context.Background(), Params{Prompt: "work", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return current.Status == "running" })
	resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))
	close(release)
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	for _, args := range []string{
		fmt.Sprintf(`{"agent_id":%q}`, r.ID),
		fmt.Sprintf(`{"agent_id":%q,"wait":true}`, r.ID),
	} {
		result := resultTool(t, m, args)
		if got := resultToolText(result); got != "kept" {
			t.Fatalf("terminal read through %s returned %q", args, got)
		}
	}
}

// A stopped child keeps its canceled outcome and partial text inside the
// requested delivery, identical to a terminal read.
func TestResultRequestDeliversCanceledOutcome(t *testing.T) {
	notes := &resultRequestNotify{}
	started := make(chan struct{})
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), SessionID: "cancel-request", Notify: notes.send,
		Create: func(context.Context, Invocation) (Child, error) {
			return &fakeChild{send: func(ctx context.Context, _ string) (string, error) {
				close(started)
				<-ctx.Done()
				return "partial work", ctx.Err()
			}}, nil
		}})
	t.Cleanup(m.Close)
	if err := m.Update(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	r, err := m.Spawn(context.Background(), Params{Prompt: "work", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))
	if err := m.Stop(r.ID, ""); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	m.flushNotifications()
	notesAll := notes.all()
	if len(notesAll) != 1 {
		t.Fatalf("deliveries: %d", len(notesAll))
	}
	_, status, outcome, body, requested := notificationResult(t, notesAll[0])
	if status != "stopped" || outcome != string(OutcomeCanceled) || !requested {
		t.Fatalf("envelope: %s %s requested=%v", status, outcome, requested)
	}
	if !strings.Contains(body, "subagent_outcome: canceled") || !strings.Contains(body, "partial work") {
		t.Fatalf("canceled delivery lost its outcome or partial text: %q", body)
	}
}

// A child the hard turn budget stops delivers its exhausted outcome and last
// answer through the same requested delivery.
func TestResultRequestDeliversExhaustedOutcome(t *testing.T) {
	claimed := make(chan struct{})
	child := &fakeChild{}
	child.send = func(ctx context.Context, _ string) (string, error) {
		<-claimed
		child.mu.Lock()
		listener := child.listener
		child.mu.Unlock()
		for range 6 {
			listener(agent.TurnStartEvent{})
		}
		<-ctx.Done()
		return "last usable answer", ctx.Err()
	}
	notes := &resultRequestNotify{}
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), SessionID: "exhausted-request", Notify: notes.send,
		Create: func(context.Context, Invocation) (Child, error) { return child, nil }})
	t.Cleanup(m.Close)
	if err := m.Update(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	maxTurns := 1
	r, err := m.Spawn(context.Background(), Params{Prompt: "work", MaxTurns: &maxTurns, Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return current.Status == "running" })
	resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))
	close(claimed)
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	m.flushNotifications()
	notesAll := notes.all()
	if len(notesAll) != 1 {
		t.Fatalf("deliveries: %d", len(notesAll))
	}
	_, status, outcome, body, requested := notificationResult(t, notesAll[0])
	if status != "exhausted" || outcome != string(OutcomeExhausted) || !requested {
		t.Fatalf("envelope: %s %s requested=%v", status, outcome, requested)
	}
	if !strings.Contains(body, "subagent_outcome: exhausted") || !strings.Contains(body, "last usable answer") {
		t.Fatalf("exhausted delivery lost its outcome or partial text: %q", body)
	}
}

// A headless caller relies on WaitResultDeliveries to stay alive until every
// claimed result has been dispatched or read terminally.
func TestWaitResultDeliveries(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	notes := &resultRequestNotify{}
	m := resultRequestManager(t, notes, release, func(p string) (string, error) {
		return fmt.Sprintf("result-%d", calls.Add(1)), nil
	})
	r, err := m.Spawn(context.Background(), Params{Prompt: "first", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return current.Status == "running" })
	resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))
	if !m.ResultDeliveriesOutstanding() {
		t.Fatal("claim did not register an outstanding delivery")
	}
	done := make(chan error, 1)
	go func() { done <- m.WaitResultDeliveries(context.Background()) }()
	select {
	case <-done:
		t.Fatal("deliveries resolved while the claim was still outstanding")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	m.flushNotifications()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("delivered claim did not resolve the waiter")
	}
	if m.ResultDeliveriesOutstanding() {
		t.Fatal("delivered claim still reads as outstanding")
	}
}

func TestWaitResultDeliveriesWakesAllWaiters(t *testing.T) {
	release := make(chan struct{})
	notes := &resultRequestNotify{}
	m := resultRequestManager(t, notes, release, func(string) (string, error) { return "exact", nil })
	r, err := m.Spawn(t.Context(), Params{Prompt: "work", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	const waiters = 8
	done := make(chan error, waiters)
	var wg sync.WaitGroup
	wg.Add(waiters)
	for range waiters {
		go func() {
			defer wg.Done()
			done <- m.WaitResultDeliveries(ctx)
		}()
	}
	close(release)
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	if got := resultToolText(resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))); got != "exact" {
		t.Fatalf("terminal read returned %q", got)
	}
	wg.Wait()
	for range waiters {
		if err := <-done; err != nil {
			t.Fatalf("terminal read did not wake a delivery waiter: %v", err)
		}
	}
	m.flushNotifications()
	if got := notes.all(); len(got) != 0 {
		t.Fatalf("a consumed result still delivered: %v", got)
	}
}

func TestResultRequestConsumedGenerationStaysConsumedAfterResume(t *testing.T) {
	release := make(chan struct{})
	notes := &resultRequestNotify{}
	m := resultRequestManager(t, notes, release, func(p string) (string, error) { return p, nil })
	r, err := m.Spawn(t.Context(), Params{Prompt: "first", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))
	close(release)
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	if got := resultToolText(resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))); got != "first" {
		t.Fatalf("terminal read returned %q", got)
	}
	if _, err := m.Spawn(t.Context(), Params{Prompt: "second", Resume: r.ID, Background: new(true)}, "", ""); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	m.flushNotifications()
	for _, note := range notes.all() {
		if strings.Contains(note, "<requested>true</requested>") || strings.Contains(note, "<result>first</result>") {
			t.Fatalf("resuming redelivered an already read result: %s", note)
		}
	}
}

func TestResultRequestRejectsCanceledCallerBeforeRegistration(t *testing.T) {
	release := make(chan struct{})
	notes := &resultRequestNotify{}
	m := resultRequestManager(t, notes, release, func(string) (string, error) { return "done", nil })
	r, err := m.Spawn(t.Context(), Params{Prompt: "work", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	tool := Tool{Manager: m, ToolName: "get_subagent_result"}
	args := fmt.Sprintf(`{"agent_id":%q}`, r.ID)
	if _, err := tool.Execute(ctx, "canceled", []byte(args), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller returned %v", err)
	}
	if got := resultToolText(resultTool(t, m, args)); strings.Contains(got, "already") {
		t.Fatalf("canceled caller registered a result demand: %s", got)
	}
	close(release)
}

func TestWaitResultDeliveriesIncludesNotificationHandoff(t *testing.T) {
	release := make(chan struct{})
	notifyStarted := make(chan struct{})
	releaseNotify := make(chan struct{})
	notes := &resultRequestNotify{}
	m := resultRequestManager(t, notes, release, func(string) (string, error) { return "done", nil })
	m.host.Notify = func(text string) {
		close(notifyStarted)
		select {
		case <-releaseNotify:
			notes.send(text)
		case <-m.ctx.Done():
		}
	}
	r, err := m.Spawn(t.Context(), Params{Prompt: "work", Background: new(true)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	resultTool(t, m, fmt.Sprintf(`{"agent_id":%q}`, r.ID))
	close(release)
	await(t, func() bool { current, _ := m.Find(r.ID); return !current.Active() })
	flushed := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(flushed)
		m.flushNotifications()
	}()
	<-notifyStarted
	if !m.ResultDeliveriesOutstanding() {
		t.Fatal("delivery resolved before the host accepted the notification")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		done <- m.WaitResultDeliveries(ctx)
	}()
	select {
	case err := <-done:
		t.Fatalf("wait returned before notification handoff: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseNotify)
	<-flushed
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if len(notes.all()) != 1 {
		t.Fatal("host did not receive the requested result")
	}
}
