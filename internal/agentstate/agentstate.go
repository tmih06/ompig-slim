// Package agentstate publishes one advisory JSON document per running agent.
// Documents live under <PIG_HOME|~/.pi>/state/agent/<pid>/status.json and are
// replaced atomically. Readers can inspect one process with [PathForPID] or
// enumerate live processes with [List].
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

// Version is the document schema version. Consumers must ignore unknown fields
// when reading a higher version.
const Version = 1

// State is the agent's externally visible lifecycle state.
type State string

const (
	Idle    State = "idle"
	Working State = "working"
	Blocked State = "blocked"
	Exited  State = "exited"
)

// Document is the published state. JSON field names are stable wire names.
type Document struct {
	Version     int    `json:"version"`
	State       State  `json:"state"`
	Agent       string `json:"agent"`
	SessionID   string `json:"session_id,omitempty"`
	SessionFile string `json:"session_file,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	Mode        string `json:"mode,omitempty"`
	Detail      string `json:"detail,omitempty"`
	BlockedKind string `json:"blocked_kind,omitempty"`
	ProcessID   int    `json:"pid"`
	UpdatedAt   string `json:"updated_at"`
}

// HeartbeatAfter defines document expiration. Readers must treat an older
// updated_at as unknown rather than current.
const HeartbeatAfter = time.Minute

// Stale reports whether doc is older than HeartbeatAfter. Missing timestamps are
// stale.
func Stale(doc Document) bool { return staleAt(doc, time.Now()) }

// staleAt reports staleness at now, allowing one second for RFC 3339 rounding.
func staleAt(doc Document, now time.Time) bool {
	updated, err := time.Parse(time.RFC3339Nano, doc.UpdatedAt)
	if err != nil {
		return true
	}
	return now.Sub(updated) > HeartbeatAfter+time.Second
}

// Root returns the directory holding all agent state documents.
func Root() (string, error) {
	root, err := configroot.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state", "agent"), nil
}

// Path returns this process's state document path. It resolves the root on each call
// so tests can change PIG_HOME.
func Path() (string, error) { return PathForPID(os.Getpid()) }

// PathForPID returns the state document path for pid.
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

// Refresh rewrites the last published document with a current timestamp. It is a
// no-op until Publish has written a document.
func (p *Publisher) Refresh() {
	if !p.Enabled() {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.last == nil {
		return
	}
	doc := *p.last
	doc.Version = Version
	doc.ProcessID = p.pid
	doc.UpdatedAt = p.clock().Format(time.RFC3339Nano)
	p.last = &doc
	p.err = p.write(doc)
	p.writes++
}

// HeartbeatInterval is how often a running session calls [Publisher.Refresh]:
// comfortably inside [HeartbeatAfter], so one missed tick still leaves a
// document a reader accepts.
const HeartbeatInterval = HeartbeatAfter / 3

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

// Read parses the document at path. A missing file returns ErrNoState; malformed
// JSON returns its decode error.
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

// List returns every live agent document under Root. Dead process directories and
// unreadable documents are removed or skipped.
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

// Live reports whether the process described by doc is running. Processes that
// cannot be signaled due to permissions are treated as alive.
func Live(doc Document) bool { return processAlive(doc.ProcessID) }
