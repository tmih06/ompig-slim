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

func readDocument(path string) (statusDocument, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return statusDocument{}, false
	}
	var doc statusDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return statusDocument{}, false
	}
	return doc, true
}

// A document is forwarded only while its writer is publishing and its state has a
// Herdr equivalent. "exited" has none on purpose: Herdr clears a pane whose agent
// returned to its shell, and a stale exit from a previous session in the same pane
// must not overwrite a live one. An unknown state is not this adapter's to invent.
func (d statusDocument) report(now time.Time) (reported, bool) {
	if !d.fresh(now) {
		return reported{}, false
	}
	state, ok := herdrState(d.State)
	if !ok {
		return reported{}, false
	}
	detail := ""
	if state == "blocked" {
		// A message belongs to the blocked state; Herdr ignores it otherwise and
		// a stale title would misdescribe an idle pane.
		detail = oneLine(d.Detail)
	}
	return reported{
		state:       state,
		detail:      detail,
		blockedKind: d.BlockedKind,
		sessionFile: d.SessionFile,
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
