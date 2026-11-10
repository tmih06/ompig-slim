package agentstateext

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/coding/extension/factoryload"
	"github.com/MichaelKinsy/PiG/coding/extension/host/inproc"

	"ompig-slim/internal/agentstate"
)

// testSessionManager is the read-only session manager view a handler sees,
// reduced to the members this package reads.
type testSessionManager struct {
	extension.ReadonlySessionManager
	id   string
	file string
}

func (m *testSessionManager) GetSessionId() string { return m.id }

func (m *testSessionManager) GetSessionFile() *string {
	file := m.file
	return &file
}

// harness dispatches real extension events through a real runner, so the
// handlers see the extension Context the CLI attaches: ctx.IsIdle(),
// ctx.SessionManager(), ctx.CWD(), and ctx.Mode() answer from the runner's
// bound actions instead of from a stub this package invents.
type harness struct {
	runner  *inproc.Runner
	root    string
	tracker *tracker
}

func newHarness(t *testing.T, idle bool, mode extension.ExtensionMode) *harness {
	t.Helper()
	root := t.TempDir()
	return newHarnessAt(t, root, idle, mode)
}

func newHarnessAt(t *testing.T, root string, idle bool, mode extension.ExtensionMode) *harness {
	t.Helper()
	publisher := agentstate.NewPublisherAt(root)
	tracked := newTracker(publisher)
	loaded, err := factoryload.LoadExtensionFromFactory(
		extensionWithTracker(publisher, tracked), root, extension.CreateEventBus(), extension.CreateExtensionRuntime(), "<inline:agentstate>",
	)
	if err != nil {
		t.Fatalf("load extension: %v", err)
	}
	runner := inproc.NewRunner([]extension.Extension{loaded}, root)
	runner.BindCore(extension.ExtensionActions{}, extension.ContextActions{
		IsIdle: func() bool { return idle },
		SessionManager: &testSessionManager{
			id:   "session-1",
			file: filepath.Join(root, "sessions", "s.jsonl"),
		},
	}, nil)
	runner.SetUIContext(extension.NoopUIContext, mode)
	return &harness{runner: runner, root: root, tracker: tracked}
}

func (h *harness) emit(t *testing.T, event extension.ExtensionEvent) {
	t.Helper()
	if _, err := h.runner.Emit(context.Background(), event); err != nil {
		t.Fatalf("emit %T: %v", event, err)
	}
}

// document reads the published state.
func (h *harness) document(t *testing.T) agentstate.Document {
	t.Helper()
	doc, err := agentstate.Read(filepath.Join(h.root, "status.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	return doc
}

func TestSessionStartIdentifiesTheSession(t *testing.T) {
	h := newHarness(t, true, extension.ModeTUI)
	h.emit(t, extension.SessionStartEvent{Type: "session_start", Reason: "startup"})

	doc := h.document(t)
	if doc.State != agentstate.Idle {
		t.Fatalf("state = %q, want idle", doc.State)
	}
	if doc.Agent != agentName {
		t.Fatalf("agent = %q", doc.Agent)
	}
	if doc.SessionID != "session-1" {
		t.Fatalf("session_id = %q", doc.SessionID)
	}
	if doc.SessionFile != filepath.Join(h.root, "sessions", "s.jsonl") {
		t.Fatalf("session_file = %q", doc.SessionFile)
	}
	if doc.Cwd != h.root {
		t.Fatalf("cwd = %q, want %q", doc.Cwd, h.root)
	}
	if doc.Mode != "tui" {
		t.Fatalf("mode = %q, want tui", doc.Mode)
	}
}

func TestRunReportsWorkingThenIdle(t *testing.T) {
	h := newHarness(t, true, extension.ModeTUI)
	h.emit(t, extension.SessionStartEvent{Type: "session_start", Reason: "startup"})

	h.emit(t, extension.AgentStartEvent{Type: "agent_start"})
	if got := h.document(t).State; got != agentstate.Working {
		t.Fatalf("during the run = %q, want working", got)
	}

	h.emit(t, extension.AgentSettledEvent{Type: "agent_settled"})
	if got := h.document(t).State; got != agentstate.Idle {
		t.Fatalf("after settle = %q, want idle", got)
	}
}

func TestBlockedDialogWinsOverARunningTurn(t *testing.T) {
	h := newHarness(t, true, extension.ModeTUI)
	h.emit(t, extension.SessionStartEvent{Type: "session_start", Reason: "startup"})
	h.emit(t, extension.AgentStartEvent{Type: "agent_start"})

	h.emit(t, extension.UIPromptStartEvent{Type: "ui_prompt_start", Kind: extension.UIPromptKindConfirm, Title: "Allow write to /tmp?"})
	doc := h.document(t)
	if doc.State != agentstate.Blocked {
		t.Fatalf("state = %q, want blocked", doc.State)
	}
	if doc.Detail != "Allow write to /tmp?" {
		t.Fatalf("detail = %q", doc.Detail)
	}
	if doc.BlockedKind != "permission" && doc.BlockedKind != "question" {
		t.Fatalf("blocked_kind = %q", doc.BlockedKind)
	}

	h.emit(t, extension.UIPromptEndEvent{Type: "ui_prompt_end", Kind: extension.UIPromptKindConfirm})
	// The dialog closed but the run continues, so the state returns to working
	// rather than idle.
	if got := h.document(t).State; got != agentstate.Working {
		t.Fatalf("after the dialog = %q, want working", got)
	}
	if got := h.document(t).Detail; got != "" {
		t.Fatalf("detail = %q after the dialog closed", got)
	}
}

func TestNestedDialogsKeepTheOutermostState(t *testing.T) {
	h := newHarness(t, true, extension.ModeTUI)
	h.emit(t, extension.SessionStartEvent{Type: "session_start", Reason: "startup"})

	h.emit(t, extension.UIPromptStartEvent{Type: "ui_prompt_start", Kind: extension.UIPromptKindSelect, Title: "outer"})
	if got := h.document(t).Detail; got != "outer" {
		t.Fatalf("detail = %q", got)
	}
	h.emit(t, extension.UIPromptStartEvent{Type: "ui_prompt_start", Kind: extension.UIPromptKindInput, Title: "inner"})
	if got := h.document(t).Detail; got != "inner" {
		t.Fatalf("detail = %q", got)
	}

	h.emit(t, extension.UIPromptEndEvent{Type: "ui_prompt_end", Kind: extension.UIPromptKindInput})
	// One dialog is still open, so the agent stays blocked with that dialog's
	// title rather than reporting idle.
	doc := h.document(t)
	if doc.State != agentstate.Blocked {
		t.Fatalf("state = %q with a dialog still open", doc.State)
	}

	h.emit(t, extension.UIPromptEndEvent{Type: "ui_prompt_end", Kind: extension.UIPromptKindSelect})
	if got := h.document(t).State; got != agentstate.Idle {
		t.Fatalf("after the last dialog = %q, want idle", got)
	}
}

func TestReloadKeepsAWorkingRunVisible(t *testing.T) {
	// A reload rebinds the extension mid-run; the session's own idle flag is
	// what keeps the published state honest.
	h := newHarness(t, false, extension.ModeTUI)
	h.emit(t, extension.SessionStartEvent{Type: "session_start", Reason: "reload"})

	if got := h.document(t).State; got != agentstate.Working {
		t.Fatalf("state = %q, want working", got)
	}
}

func TestShutdownPublishesTheTerminalState(t *testing.T) {
	h := newHarness(t, true, extension.ModeTUI)
	h.emit(t, extension.SessionStartEvent{Type: "session_start", Reason: "startup"})
	h.emit(t, extension.AgentStartEvent{Type: "agent_start"})

	h.emit(t, extension.SessionShutdownEvent{Type: "session_shutdown", Reason: "quit"})
	if got := h.document(t).State; got != agentstate.Exited {
		t.Fatalf("state = %q, want exited", got)
	}

	// A late event must not resurrect the session.
	h.emit(t, extension.AgentStartEvent{Type: "agent_start"})
	if got := h.document(t).State; got != agentstate.Exited {
		t.Fatalf("state = %q after a late event", got)
	}
}

// TestSessionScopedShutdownKeepsPublishing pins the difference between a process
// that is ending and one that replaced its session.
//
// `session_shutdown` does not mean the process is gone: `/clear` emits it with
// reason "new", `/reload` with "reload", and resuming or forking with their own
// reasons. Treating every shutdown as terminal published `exited` and then
// ignored every later event, so a live agent's document claimed it had exited for
// the rest of the process. A consumer reads `exited` as "stop watching this pane",
// which is exactly the reported symptom: the multiplexer showed no agent even
// though the agent was working.
func TestSessionScopedShutdownKeepsPublishing(t *testing.T) {
	for _, reason := range []string{"new", "reload", "resume", "fork"} {
		t.Run(reason, func(t *testing.T) {
			h := newHarness(t, true, extension.ModeTUI)
			h.emit(t, extension.SessionStartEvent{Type: "session_start", Reason: "startup"})
			h.emit(t, extension.AgentStartEvent{Type: "agent_start"})
			if got := h.document(t).State; got != agentstate.Working {
				t.Fatalf("state = %q before the switch, want working", got)
			}

			h.emit(t, extension.SessionShutdownEvent{Type: "session_shutdown", Reason: reason})
			// The ended session's run is over; the process is not.
			if got := h.document(t).State; got != agentstate.Idle {
				t.Fatalf("state = %q after a %q shutdown, want idle", got, reason)
			}

			// The next session of the same process must still publish.
			h.emit(t, extension.AgentStartEvent{Type: "agent_start"})
			if got := h.document(t).State; got != agentstate.Working {
				t.Fatalf("state = %q after the next agent_start, want working", got)
			}
			h.emit(t, extension.AgentSettledEvent{Type: "agent_settled"})
			if got := h.document(t).State; got != agentstate.Idle {
				t.Fatalf("state = %q after the next turn, want idle", got)
			}
		})
	}
}

// TestSessionScopedShutdownClearsAStaleDialog pins that a replaced session does
// not inherit the previous one's blocking dialog: a `blocked` document with no
// dialog open would make a consumer wait on a user who is not being asked
// anything.
func TestSessionScopedShutdownClearsAStaleDialog(t *testing.T) {
	h := newHarness(t, true, extension.ModeTUI)
	h.emit(t, extension.SessionStartEvent{Type: "session_start", Reason: "startup"})
	h.emit(t, extension.UIPromptStartEvent{Type: "ui_prompt_start", Kind: "permission", Title: "Allow?"})
	if got := h.document(t).State; got != agentstate.Blocked {
		t.Fatalf("state = %q before the switch, want blocked", got)
	}

	h.emit(t, extension.SessionShutdownEvent{Type: "session_shutdown", Reason: "new"})
	document := h.document(t)
	if document.State != agentstate.Idle {
		t.Fatalf("state = %q after a /clear, want idle", document.State)
	}
	if document.Detail != "" || document.BlockedKind != "" {
		t.Fatalf("a replaced session kept the old dialog: %+v", document)
	}
}

// TestHeartbeatKeepsTheDocumentCurrent pins that a running session re-stamps its
// document, so a consumer applying the staleness rule never calls a healthy agent
// unknown while it sits idle between turns.
//
// Without the heartbeat the document keeps the timestamp of the last state change,
// which for an idle agent is arbitrarily old. That is precisely how a pane can read
// as gone: the reader sees a document it cannot prove is current.
func TestHeartbeatKeepsTheDocumentCurrent(t *testing.T) {
	h := newHarness(t, true, extension.ModeTUI)
	h.tracker.beatEvery = 10 * time.Millisecond
	h.emit(t, extension.SessionStartEvent{Type: "session_start", Reason: "startup"})

	before := h.document(t)
	if before.State != agentstate.Idle {
		t.Fatalf("state = %q, want idle", before.State)
	}

	// Shorten the loop so the test observes a beat instead of waiting a third of a
	// minute for the production interval.
	h.tracker.beatEvery = 10 * time.Millisecond
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if document := h.document(t); document.UpdatedAt != before.UpdatedAt {
			if document.State != agentstate.Idle {
				t.Fatalf("the heartbeat changed the state to %q", document.State)
			}
			if agentstate.Stale(document) {
				t.Fatal("the refreshed document counted as stale")
			}
			// A quit stops the loop, so a session that is gone stops publishing
			// instead of leaving a goroutine writing for it.
			h.emit(t, extension.SessionShutdownEvent{Type: "session_shutdown", Reason: "quit"})
			select {
			case <-h.tracker.beatStopped:
			case <-time.After(2 * time.Second):
				t.Fatal("the heartbeat outlived the session")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the document timestamp never advanced")
}

func TestHeadlessRunsPublishTheirMode(t *testing.T) {
	// A headless run is a real agent doing real work; it publishes, and the mode
	// is what lets a consumer decide whether it is the pane's interactive agent.
	for _, mode := range []extension.ExtensionMode{extension.ModePrint, extension.ModeJSON, extension.ModeRPC} {
		h := newHarness(t, true, mode)
		h.emit(t, extension.SessionStartEvent{Type: "session_start", Reason: "startup"})
		doc := h.document(t)
		if doc.Mode != string(mode) {
			t.Fatalf("mode = %q, want %q", doc.Mode, mode)
		}
	}
}

func TestDetailIsOnePrintableLine(t *testing.T) {
	h := newHarness(t, true, extension.ModeTUI)
	h.emit(t, extension.SessionStartEvent{Type: "session_start", Reason: "startup"})
	h.emit(t, extension.UIPromptStartEvent{
		Type:  "ui_prompt_start",
		Kind:  extension.UIPromptKindConfirm,
		Title: "Allow write\nto /tmp?\t",
	})

	if got, want := h.document(t).Detail, "Allow write to /tmp?"; got != want {
		t.Fatalf("detail = %q, want %q", got, want)
	}
}
