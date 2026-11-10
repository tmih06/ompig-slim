// Package agentstate publishes what this program's agent is doing so other
// programs — a terminal multiplexer, a status bar, a notifier, a test harness —
// can watch it without knowing anything about ompig-slim's internals.
//
// The surface is one JSON document per running agent under the configuration
// root, rewritten atomically whenever the state changes:
//
//	<PIG_HOME|~/.pi>/state/agent/<pid>/status.json
//
// One directory per process, not one shared file, because several agents run at
// once on a real machine: two Herdr panes, a pane plus a `-p` run from a script,
// or a subagent host. A single file would have them overwrite each other, and a
// consumer would see whichever wrote last — including, on a Herdr pane, a rival
// session's state. A process identifier is the right key because one process runs
// one session at a time; a session replacement reuses the directory.
//
// A consumer either reads the document of the process it cares about
// ([PathForPID]) or enumerates the live ones ([List]). Nothing in this package
// knows about Herdr, terminals, or any other consumer, and a consumer that does
// not exist costs one small file write per state change.
//
// # Why a file
//
// A lifecycle event is a moment, and a consumer that starts after it (a status
// bar launched later, an integration attached to an existing session) would
// miss it. Publishing the *current* state instead of the transitions means a
// consumer only has to read the file to be correct, and a publish is naturally
// coalescing: two changes that happen faster than a write leave the newest
// state on disk.
//
// # Contract
//
// The document is advisory. A reader must treat an absent file, an unreadable
// file, or an old timestamp as "no information" rather than an error, because
// the writer is a normal process that can be killed mid-turn. `HeartbeatAfter`
// in the document lets a reader distinguish "idle since long ago" from "the
// writer died", which a bare state string cannot express.
package agentstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"ompig-slim/internal/configroot"
)

// Version is the document's schema version. A consumer that sees a higher
// version should ignore fields it does not know rather than reject the file;
// the version exists so a consumer can detect a shape it cannot read at all.
const Version = 1

// State is what the agent is doing. The set is deliberately the union of what
// terminal multiplexers, status bars, and `wait`-style automation need, and no
// more: a consumer that wants nuance reads `detail`.
type State string

const (
	// Idle means the agent is waiting for input: no run is in flight, and no
	// dialog is open.
	Idle State = "idle"
	// Working means a run is in flight.
	Working State = "working"
	// Blocked means the agent cannot proceed without the user: a permission
	// dialog, a question, or a login prompt is open. `detail` names it.
	Blocked State = "blocked"
	// Exited means the program is no longer running in this directory. It is
	// written once on shutdown so a consumer stops waiting on a dead session
	// instead of trusting a stale `working`.
	Exited State = "exited"
)

// Document is the published state. Field names are stable wire names; adding a
// field is a compatible change, removing or renaming one is not.
type Document struct {
	// Version is [Version] as written by the producer.
	Version int `json:"version"`
	// State is the agent's current state.
	State State `json:"state"`
	// Agent is the program name, so a consumer watching several harnesses can
	// tell them apart: "ompig-slim".
	Agent string `json:"agent"`
	// SessionID is PiG's session identifier, stable across a run.
	SessionID string `json:"session_id,omitempty"`
	// SessionFile is the session log's path, which is also what resumes it.
	SessionFile string `json:"session_file,omitempty"`
	// Cwd is the working directory the agent runs in.
	Cwd string `json:"cwd,omitempty"`
	// Mode is the run mode: "tui", "rpc", "json", or "print". A consumer that
	// only cares about interactive agents should require "tui", because a
	// one-shot run in the same directory is not the agent a user is watching.
	Mode string `json:"mode,omitempty"`
	// Detail is one line of human-readable context for a blocked state — the
	// dialog title — and empty otherwise. Control characters are replaced so
	// the value is safe to print unescaped.
	Detail string `json:"detail,omitempty"`
	// BlockedKind is what a blocked agent waits for: "permission", "question",
	// or "auth". Empty when the producer cannot classify the dialog.
	BlockedKind string `json:"blocked_kind,omitempty"`
	// ProcessID is the writer's process ID, so a reader can tell a live
	// document from one left behind by a killed process.
	ProcessID int `json:"pid"`
	// UpdatedAt is when the document was last written, as RFC 3339 with a
	// numeric zone. A reader that finds this very old should verify
	// [Document.ProcessID] is still alive before trusting `state`.
	UpdatedAt string `json:"updated_at"`
}

// Root returns the directory holding every agent's document:
// <config root>/state/agent. A consumer that does not already know which process
// it wants enumerates this directory.
func Root() (string, error) {
	root, err := configroot.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state", "agent"), nil
}

// Path returns this process's document path. It is computed rather than cached
// so a test can point PIG_HOME at a temporary directory after this package has
// been used; confidence in the resolution does not justify process-wide state.
func Path() (string, error) { return PathForPID(os.Getpid()) }

// PathForPID returns the document path of the agent running as pid, which is what
// a consumer embedded in that agent (an extension, for instance) reads: it is
// the one document that describes the session it runs in.
func PathForPID(pid int) (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, strconv.Itoa(pid), "status.json"), nil
}

// Publisher writes the state document. It is safe for concurrent use and
// collapses bursts: a publish that would write the document already on disk is
// skipped, and publishes are serialized so a slow filesystem cannot reorder
// them.
//
// A Publisher with no destination (one whose environment has no configuration
// root, or one explicitly disabled) drops every publish. That is deliberate:
// nothing about this feature should be able to fail a user's turn.
type Publisher struct {
	// dir is the directory holding the document; empty disables publishing.
	dir string
	// pid is the process the document describes, and the name of dir. It is fixed
	// at construction so the document's ProcessID and the directory a consumer
	// enumerates can never disagree; [NewPublisher] reads the live process, and
	// [NewPublisherAt] takes it from the directory name so a supervisor can write
	// or read another process's document.
	pid int
	// clock is time.Now in production and a fixed function in tests.
	clock func() time.Time

	mu sync.Mutex
	// last is the document as written, for duplicate suppression.
	last *Document
	// writes counts completed writes; tests use it to wait for a publish.
	writes int
	// err is the last write failure, which a caller may log once.
	err error
}

// NewPublisher returns a Publisher writing this process's document under the
// configuration root. A root that cannot be resolved yields a disabled Publisher
// rather than an error, because publishing is decorative: a machine without a
// resolvable home directory can still run the agent.
func NewPublisher() *Publisher {
	path, err := Path()
	if err != nil {
		return &Publisher{}
	}
	return NewPublisherAt(filepath.Dir(path))
}

// NewPublisherAt returns a Publisher that writes status.json into dir, describing
// the process the directory is named for. An empty dir returns a disabled
// Publisher; a directory whose name is not a process id describes this process.
func NewPublisherAt(dir string) *Publisher {
	return &Publisher{dir: dir, pid: pidFromDir(dir), clock: time.Now}
}

// pidFromDir reads the process id a per-agent directory is named for, falling
// back to this process for a directory that is not named for one.
func pidFromDir(dir string) int {
	if dir == "" {
		return 0
	}
	if pid, err := strconv.Atoi(filepath.Base(dir)); err == nil && pid > 0 {
		return pid
	}
	return os.Getpid()
}

// Enabled reports whether this Publisher writes anything.
func (p *Publisher) Enabled() bool { return p != nil && p.dir != "" }

// Publish writes doc. Version, ProcessID, and UpdatedAt are filled in here so a
// caller cannot forget them.
//
// Publish reports nothing. The state document is an optimization for external
// tools: a full disk or a read-only home must not surface in the agent's UI or
// change the exit status of a turn. The last error is kept for
// [Publisher.LastError] so a caller that wants to log once can.
func (p *Publisher) Publish(doc Document) {
	if !p.Enabled() {
		return
	}
	doc.Version = Version
	doc.ProcessID = p.pid
	doc.UpdatedAt = p.clock().Format(time.RFC3339Nano)

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.last != nil && sameContent(*p.last, doc) {
		return
	}
	// Record before the write so a concurrent duplicate is dropped even if the
	// write fails; the next state change publishes the current truth anyway.
	stored := doc
	p.last = &stored
	p.err = p.write(doc)
	p.writes++
}

// LastError returns the most recent write failure, or nil.
func (p *Publisher) LastError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// Remove deletes the document and its per-process directory, so a consumer
// enumerating [Root] sees one fewer agent instead of a stale state.
//
// The directory is only removed when it is empty: a publisher that shares it
// with something else must not delete that.
func (p *Publisher) Remove() {
	if !p.Enabled() {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.last = nil
	_ = os.Remove(p.path())
	_ = os.Remove(p.dir)
}

// RemoveProcess deletes the document of the agent that ran as pid, which a
// supervisor uses to clear a crashed process's leftovers.
func RemoveProcess(pid int) error {
	path, err := PathForPID(pid)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// A directory that still holds files is somebody else's; leave it.
	_ = os.Remove(filepath.Dir(path))
	return nil
}

// write renders and atomically replaces the document: a reader never sees a
// half-written file, and a crash mid-write leaves the previous document intact.
func (p *Publisher) write(doc Document) error {
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	temporary, err := os.CreateTemp(p.dir, ".status-*.json")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() {
		// A successful rename consumed the file; removing it again is a no-op.
		_ = os.Remove(name)
	}()
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	// 0o644: a status document is not a secret. It carries the cwd and session
	// path, which every user of the account can already read, and a status bar
	// running as another process of the same user must be able to read it.
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, p.path())
}

func (p *Publisher) path() string { return filepath.Join(p.dir, "status.json") }

// sameContent reports whether two documents would render identically. The
// timestamp and process ID are excluded so a repeated publish of the same state
// does not rewrite the file, which keeps a watcher quiet while the agent sits
// idle.
func sameContent(left, right Document) bool {
	left.Version, right.Version = 0, 0
	left.ProcessID, right.ProcessID = 0, 0
	left.UpdatedAt, right.UpdatedAt = "", ""
	return left == right
}

// Read parses the document at path. A consumer uses it; tests use it to assert
// what was published.
//
// A missing file returns [ErrNoState] so a caller can distinguish "nothing has
// published here" from a corrupt document, which returns the decode error.
func Read(path string) (Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Document{}, ErrNoState
		}
		return Document{}, err
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

// ErrNoState reports that no state document exists at the requested path.
var ErrNoState = errors.New("no agent state document")

// List returns every live agent document under [Root], in no particular order.
//
// It is the entry point for a consumer that watches the machine rather than one
// known agent: a status bar showing every running session, a dashboard, or a
// supervisor looking for work to do. A document whose process is gone is skipped
// and its directory removed, because a killed writer cannot clean up after
// itself and a stale `working` in a listing is worse than a missing entry.
//
// An unreadable or corrupt document is skipped rather than failing the call: one
// bad directory must not blind a consumer to every other agent.
func List() ([]Document, error) {
	root, err := Root()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	documents := make([]Document, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		path := filepath.Join(root, entry.Name(), "status.json")
		document, err := Read(path)
		if err != nil {
			continue
		}
		if !processAlive(document.ProcessID) {
			// The writer cannot remove its own document once it is gone.
			_ = os.Remove(path)
			_ = os.Remove(filepath.Dir(path))
			continue
		}
		documents = append(documents, document)
	}
	return documents, nil
}

// Live reports whether the process that wrote doc is still running, which is how
// a consumer tells a current document from a leftover one. A consumer that only
// reads one document by path uses this; [List] already applies it.
//
// The check is deliberately conservative: a process this one may not signal
// (another user's, or a pid the platform will not report on) counts as alive, so
// a document is never dropped for lack of permission to check it.
func Live(doc Document) bool { return processAlive(doc.ProcessID) }
