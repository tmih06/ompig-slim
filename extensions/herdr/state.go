package herdr

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

// statusDocument mirrors the fields of ompig-slim's state document this adapter
// reads. It is a copy rather than an import: the extension is built against
// PiG's staged SDK, and a build that had to resolve the harness's own module
// would drag the whole runtime into it.
type statusDocument struct {
	State       string `json:"state"`
	SessionID   string `json:"session_id"`
	Detail      string `json:"detail"`
	BlockedKind string `json:"blocked_kind"`
	SessionFile string `json:"session_file"`
	UpdatedAt   string `json:"updated_at"`
}

// staleAfter is how old a document may be before it stops describing its writer.
//
// The harness refreshes the document every twenty seconds while it runs, so a
// minute old means the writer stopped publishing: an extension reloaded mid-run, a
// process suspended and resumed, a killed writer whose pid was reused. Reporting
// such a document would claim a state nothing observed — and Herdr never expires a
// reported state on its own, so a frozen `working` would show the pane as busy
// forever. Both values are the harness's documented contract; they are spelled out
// here because this module cannot import the harness.
const staleAfter = time.Minute

// fresh reports whether the document is recent enough to trust. A document with
// no readable timestamp counts as stale: guessing it is current is how a reader
// shows a dead agent as active.
func (d statusDocument) fresh(now time.Time) bool {
	updated, err := time.Parse(time.RFC3339Nano, d.UpdatedAt)
	if err != nil {
		return false
	}
	return now.Sub(updated) <= staleAfter
}

// readReported reads the state document at path and maps it onto a Herdr report.
//
// A missing, unreadable, stale or unmappable document returns false instead of an
// error: the caller polls, and a document that is briefly absent (the writer
// replacing it, the session not yet started) or no longer refreshed must not be
// reported as any state at all. Silently reporting nothing is correct because the
// last state Herdr holds is still the last thing that was true; the alternative,
// forwarding a frozen state, is what makes a pane read as busy after its agent
// stopped publishing. "exited" is not forwarded — Herdr clears a pane whose agent
// returned to its shell, and a stale exit from a previous session in the same pane
// must not overwrite a live one.
func readReported(path string) (reported, bool) {
	return readReportedAt(path, time.Now())
}

// readReportedAt is readReported against an explicit clock, so a test can age a
// document without sleeping.
func readReportedAt(path string, now time.Time) (reported, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return reported{}, false
	}
	var doc statusDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return reported{}, false
	}
	if !doc.fresh(now) {
		return reported{}, false
	}
	state, ok := herdrState(doc.State)
	if !ok {
		return reported{}, false
	}
	detail := ""
	if state == "blocked" {
		// A message belongs to the blocked state; Herdr ignores it otherwise and
		// a stale title would misdescribe an idle pane.
		detail = oneLine(doc.Detail)
	}
	return reported{
		state:       state,
		detail:      detail,
		blockedKind: doc.BlockedKind,
		sessionFile: doc.SessionFile,
	}, true
}

// herdrState maps a published state onto the set Herdr accepts. `exited` has no
// Herdr equivalent on purpose: the pane's shell prompt is Herdr's own signal for
// "the agent is gone", and inventing an idle report for it would claim a pane the
// agent no longer owns.
func herdrState(state string) (string, bool) {
	switch state {
	case "idle":
		return "idle", true
	case "working":
		return "working", true
	case "blocked":
		return "blocked", true
	default:
		return "", false
	}
}

// oneLine collapses a dialog title to a single printable line and caps it, so a
// long prompt cannot crowd out the agent name in a sidebar row.
func oneLine(text string) string {
	const limit = 120
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r < ' ' || r == '\u007f'
	})
	runes := []rune(strings.Join(fields, " "))
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return string(runes)
}
