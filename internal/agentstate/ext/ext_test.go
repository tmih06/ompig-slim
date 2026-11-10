package agentstateext

import (
	"context"
	"path/filepath"
	"testing"

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
	runner *inproc.Runner
	root   string
}

func newHarness(t *testing.T, idle bool, mode extension.ExtensionMode) *harness {
	t.Helper()
	root := t.TempDir()
	return newHarnessAt(t, root, idle, mode)
}

func newHarnessAt(t *testing.T, root string, idle bool, mode extension.ExtensionMode) *harness {
	t.Helper()
	publisher := agentstate.NewPublisherAt(root)
	loaded, err := factoryload.LoadExtensionFromFactory(
		Extension(publisher), root, extension.CreateEventBus(), extension.CreateExtensionRuntime(), "<inline:agentstate>",
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
	return &harness{runner: runner, root: root}
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
