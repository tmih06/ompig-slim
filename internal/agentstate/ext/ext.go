// Package agentstateext publishes agentstate documents from PiG session events.
package agentstateext

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/coding/extension"

	"ompig-slim/internal/agentstate"
)

// agentName is the program identifier published in agentstate documents.
const agentName = "ompig-slim"

// Extension returns the PiG extension factory that publishes agent state.
func Extension(publisher *agentstate.Publisher) extension.ExtensionFactory {
	return extensionWithTracker(publisher, nil)
}

// extensionWithTracker constructs a factory with an optional test tracker.
func extensionWithTracker(publisher *agentstate.Publisher, provided *tracker) extension.ExtensionFactory {
	return func(api extension.API) error {
		session := provided
		if session == nil {
			session = newTracker(publisher)
		}

		api.OnSessionStart(func(ctx context.Context, _ extension.SessionStartEvent) error {
			session.identify(ctx)
			// Read runner idle state so a mid-run reload preserves the status.
			session.setRunning(!session.idle(ctx))
			session.startHeartbeat()
			return nil
		})
		api.OnAgentStart(func(_ context.Context, _ extension.AgentStartEvent) error {
			session.setRunning(true)
			return nil
		})
		api.OnAgentSettled(func(_ context.Context, _ extension.AgentSettledEvent) error {
			// agent_settled fires after retries and continuations complete.
			session.setRunning(false)
			return nil
		})
		api.OnUiPromptStart(func(_ context.Context, event extension.UIPromptStartEvent) error {
			session.block(string(event.Kind), event.Title)
			return nil
		})
		api.OnUiPromptEnd(func(_ context.Context, _ extension.UIPromptEndEvent) error {
			session.unblock()
			return nil
		})
		api.OnSessionShutdown(func(_ context.Context, event extension.SessionShutdownEvent) error {
			session.shutdown(event.Reason)
			return nil
		})
		return nil
	}
}

// tracker serializes session lifecycle events and publishes under mu.
type tracker struct {
	publisher *agentstate.Publisher

	mu          sync.Mutex
	sessionID   string
	sessionFile string
	cwd         string
	mode        string
	running     bool
	// blocked tracks open dialog count, detail, and kind.
	blocked   int
	detail    string
	kind      string
	exited    bool
	heartbeat chan struct{}
	// beatEvery overrides HeartbeatInterval in tests.
	beatEvery   time.Duration
	beatStopped chan struct{}
}

func newTracker(publisher *agentstate.Publisher) *tracker {
	return &tracker{publisher: publisher}
}

// startHeartbeat keeps the document's timestamp current for as long as the
// process publishes, so a reader applying [agentstate.HeartbeatAfter] never calls
// a healthy agent stale. It is idempotent: a repeated session_start (a reload)
// keeps the running loop instead of starting a second one, and a session whose
// document has not been published yet starts nothing.
func (t *tracker) startHeartbeat() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.heartbeat != nil {
		return
	}
	t.heartbeat = make(chan struct{})
	t.beatStopped = make(chan struct{})
	every := t.beatEvery
	if every <= 0 {
		every = agentstate.HeartbeatInterval
	}
	go t.beat(t.heartbeat, t.beatStopped, every)
}

func (t *tracker) beat(done <-chan struct{}, stopped chan<- struct{}, every time.Duration) {
	defer close(stopped)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
		}
		t.mu.Lock()
		if t.exited {
			t.mu.Unlock()
			return
		}
		// Hold mu while refreshing to serialize it with state changes.
		t.publisher.Refresh()
		t.mu.Unlock()
	}
}

// identify reads and records session metadata. Metadata failures leave fields empty.
func (t *tracker) identify(ctx context.Context) {
	sessionID, sessionFile, cwd, mode := sessionIdentity(ctx)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessionID, t.sessionFile, t.cwd, t.mode = sessionID, sessionFile, cwd, mode
	t.publishLocked()
}

// idle reports whether the runner is idle, defaulting to true when unavailable.
func (t *tracker) idle(ctx context.Context) bool {
	extCtx := extension.FromContext(ctx)
	if extCtx == nil {
		return true
	}
	idle, err := extCtx.IsIdle()
	if err != nil {
		return true
	}
	return idle
}

func (t *tracker) setRunning(running bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.exited {
		return
	}
	t.running = running
	t.publishLocked()
}

func (t *tracker) block(kind, detail string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.exited {
		return
	}
	t.blocked++
	t.kind = normalizeKind(kind)
	t.detail = oneLine(detail)
	t.publishLocked()
}

func (t *tracker) unblock() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.exited {
		return
	}
	if t.blocked > 0 {
		t.blocked--
	}
	if t.blocked == 0 {
		t.kind, t.detail = "", ""
	}
	t.publishLocked()
}

// exit publishes the terminal exited state for process shutdown.
func (t *tracker) exit() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.exited {
		return
	}
	t.exited = true
	t.running = false
	t.blocked = 0
	if t.heartbeat != nil {
		close(t.heartbeat)
		t.heartbeat = nil
	}
	t.publishLocked()
}

// shutdown distinguishes process exit from session reset. Non-terminal shutdowns
// clear transient state so the next session can publish fresh metadata.
func (t *tracker) shutdown(reason string) {
	switch reason {
	case "quit", "":
		t.exit()
	default:
		t.reset()
	}
}

// reset clears run and dialog state for a replaced session and republishes.
func (t *tracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.exited {
		return
	}
	t.running = false
	t.blocked = 0
	t.kind, t.detail = "", ""
	t.publishLocked()
}

// publishLocked publishes the derived state. Caller holds t.mu.
//
// The blocked state wins over a running one because a dialog can open mid-run
// and is exactly what a consumer waiting for the user must see.
func (t *tracker) publishLocked() {
	state := agentstate.Idle
	switch {
	case t.exited:
		state = agentstate.Exited
	case t.blocked > 0:
		state = agentstate.Blocked
	case t.running:
		state = agentstate.Working
	}
	document := agentstate.Document{
		State:       state,
		Agent:       agentName,
		SessionID:   t.sessionID,
		SessionFile: t.sessionFile,
		Cwd:         t.cwd,
		Mode:        t.mode,
		Detail:      t.detail,
		BlockedKind: t.kind,
	}
	t.publisher.Publish(document)
}

// sessionIdentity reads the session facts a consumer may want. Each is optional;
// a host that cannot answer leaves the field empty.
func sessionIdentity(ctx context.Context) (sessionID, sessionFile, cwd, mode string) {
	extCtx := extension.FromContext(ctx)
	if extCtx == nil {
		return "", "", "", ""
	}
	if manager, err := extCtx.SessionManager(); err == nil && manager != nil {
		sessionID = manager.GetSessionId()
		if file := manager.GetSessionFile(); file != nil {
			sessionFile = *file
		}
	}
	if dir, err := extCtx.CWD(); err == nil {
		cwd = dir
	}
	if selected, err := extCtx.Mode(); err == nil {
		mode = string(selected)
	}
	return sessionID, sessionFile, cwd, mode
}

// normalizeKind maps PiG's dialog kinds onto the documented set. An unknown kind
// becomes empty rather than leaking an unclassified value into the schema.
func normalizeKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "permission":
		return "permission"
	case "question", "select", "confirm", "input", "editor", "custom":
		return "question"
	case "auth":
		return "auth"
	default:
		return ""
	}
}

// oneLine removes control characters and caps the title at 160 runes.
func oneLine(text string) string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r < ' ' || r == '\u007f'
	})
	line := strings.Join(fields, " ")
	const limit = 160
	if runes := []rune(line); len(runes) > limit {
		return string(runes[:limit])
	}
	return line
}
