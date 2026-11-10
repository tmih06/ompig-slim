// Package agentstateext is the extension that publishes [agentstate] documents
// from a live PiG session.
//
// It is the only part of the state surface that must run inside the session,
// because the events it needs are emitted by PiG's runner. Everything a consumer
// needs — the file, its schema, and a reader — lives in [agentstate], so the
// Herdr integration, a status bar, and a test harness all read the same
// document instead of each learning this extension's protocol.
package agentstateext

import (
	"context"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/coding/extension"

	"ompig-slim/internal/agentstate"
)

// agentName is the program name published in the document, and the identity a
// consumer keys on to tell this harness from another.
const agentName = "ompig-slim"

// Extension returns the factory that publishes agent state. `publisher` may be
// nil or disabled, in which case the handlers do nothing at all.
//
// The Factory shape — registering event handlers only — means the extension
// adds no tool, command, or flag to the session, so it cannot shadow a user's
// extension or appear in the tool list.
func Extension(publisher *agentstate.Publisher) extension.ExtensionFactory {
	return func(api extension.API) error {
		session := newTracker(publisher)

		api.OnSessionStart(func(ctx context.Context, _ extension.SessionStartEvent) error {
			session.identify(ctx)
			// A reload replaces the extension mid-run, so the session's own
			// idle flag is the only trustworthy answer here; assuming "idle"
			// would report a working agent as ready for input.
			session.setRunning(!session.idle(ctx))
			return nil
		})
		api.OnAgentStart(func(_ context.Context, _ extension.AgentStartEvent) error {
			session.setRunning(true)
			return nil
		})
		api.OnAgentSettled(func(_ context.Context, _ extension.AgentSettledEvent) error {
			// Settled, not agent_end: PiG fires this only once retries,
			// compaction, and queued continuations are done, so a consumer does
			// not see "idle" between two attempts of the same turn.
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
		api.OnSessionShutdown(func(_ context.Context, _ extension.SessionShutdownEvent) error {
			session.exit()
			return nil
		})
		return nil
	}
}

// tracker derives the published document from the session's lifecycle.
//
// The handlers run on different goroutines — prompt events are dispatched from
// the runner's own drain goroutine while run events come from the run loop — so
// every field is mutex-guarded and each mutation publishes under the lock. That
// keeps the file's order equal to the events' order.
type tracker struct {
	publisher *agentstate.Publisher

	mu sync.Mutex
	// identified fields describe the session, refreshed at session_start.
	sessionID   string
	sessionFile string
	cwd         string
	mode        string
	// running is true between agent_start and agent_settled.
	running bool
	// blocked counts the open blocking dialogs; detail and kind describe the
	// newest one. A nested dialog raises the count without changing what the
	// consumer sees, so only the outermost close ends "blocked".
	blocked int
	detail  string
	kind    string
	// exited stops further publishing once the session has shut down.
	exited bool
}

func newTracker(publisher *agentstate.Publisher) *tracker {
	return &tracker{publisher: publisher}
}

// identify records the session's identity. Failures are ignored: a document
// without a session path is still useful, and the alternative is failing the
// user's session_start handler over metadata.
func (t *tracker) identify(ctx context.Context) {
	sessionID, sessionFile, cwd, mode := sessionIdentity(ctx)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessionID, t.sessionFile, t.cwd, t.mode = sessionID, sessionFile, cwd, mode
	t.publishLocked()
}

// idle reads the session's own idle flag. A missing context or a stale runner
// reports idle, which is the safe answer: claiming "working" would make a
// consumer wait on a run that is not happening.
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

// exit publishes the terminal state once. A consumer that keeps polling learns
// the agent is gone instead of trusting the last `working` forever.
func (t *tracker) exit() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.exited {
		return
	}
	t.exited = true
	t.running = false
	t.blocked = 0
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

// oneLine collapses a dialog title to a single printable line and caps it, so a
// consumer can print the value without escaping and without a long prompt
// crowding out its own layout.
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
