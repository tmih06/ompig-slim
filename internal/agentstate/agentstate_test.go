package agentstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// readDocument reads the document a publisher wrote into dir.
func readDocument(t *testing.T, dir string) Document {
	t.Helper()
	doc, err := Read(filepath.Join(dir, "status.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	return doc
}

func TestPublishWritesTheDocumentAtTheFixedPath(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "state", "agent")

	publisher := NewPublisherAt(root)
	publisher.Publish(Document{State: Working, Agent: "ompig-slim"})

	doc := readDocument(t, root)
	if doc.State != Working || doc.Agent != "ompig-slim" {
		t.Fatalf("document = %+v", doc)
	}
	if doc.Version != Version {
		t.Fatalf("version = %d, want %d", doc.Version, Version)
	}
	if doc.ProcessID != os.Getpid() {
		t.Fatalf("pid = %d, want %d", doc.ProcessID, os.Getpid())
	}
	if _, err := time.Parse(time.RFC3339Nano, doc.UpdatedAt); err != nil {
		t.Fatalf("updated_at %q is not RFC 3339: %v", doc.UpdatedAt, err)
	}
}

func TestPathIsPerProcessUnderTheConfigurationRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PIG_HOME", root)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(root, "state", "agent", strconv.Itoa(os.Getpid()), "status.json")
	if path != want {
		t.Fatalf("Path() = %q, want %q", path, want)
	}

	other, err := PathForPID(4242)
	if err != nil {
		t.Fatalf("PathForPID: %v", err)
	}
	if other == path {
		t.Fatal("two processes must not share a document")
	}
}

func TestPublisherFollowsPigHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PIG_HOME", root)

	publisher := NewPublisher()
	if !publisher.Enabled() {
		t.Fatal("a resolvable configuration root must enable publishing")
	}
	publisher.Publish(Document{State: Idle, Agent: "ompig-slim"})

	doc := readDocument(t, filepath.Join(root, "state", "agent", strconv.Itoa(os.Getpid())))
	if doc.State != Idle {
		t.Fatalf("state = %q", doc.State)
	}
}

func TestPublishIsAtomicAndNeverLeavesAPartialDocument(t *testing.T) {
	root := t.TempDir()
	publisher := NewPublisherAt(root)
	publisher.Publish(Document{State: Idle, Agent: "ompig-slim"})

	// A reader running concurrently must always see a complete document.
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(filepath.Join(root, "status.json"))
			if err != nil {
				continue
			}
			var doc Document
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Errorf("read a partial document: %v", err)
				return
			}
			if doc.State == "" {
				t.Error("read a document with no state")
				return
			}
		}
	}()
	for i := 0; i < 40; i++ {
		state := Working
		if i%2 == 0 {
			state = Idle
		}
		publisher.Publish(Document{State: state, Agent: "ompig-slim"})
	}
	close(stop)
	wg.Wait()
}

func TestPublishSkipsARepeatedDocument(t *testing.T) {
	root := t.TempDir()
	publisher := NewPublisherAt(root)
	publisher.Publish(Document{State: Idle, Agent: "ompig-slim"})

	path := filepath.Join(root, "status.json")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	// Repeating the same content must not rewrite the file: a watcher would
	// wake for a change that did not happen, and the timestamp would lie about
	// when the state last changed.
	publisher.Publish(Document{State: Idle, Agent: "ompig-slim"})
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("an identical document was rewritten: %v -> %v", before.ModTime(), after.ModTime())
	}

	// A real change still writes.
	publisher.Publish(Document{State: Working, Agent: "ompig-slim"})
	if got := readDocument(t, root).State; got != Working {
		t.Fatalf("state = %q, want working", got)
	}
}

func TestPublishKeepsEveryField(t *testing.T) {
	root := t.TempDir()
	publisher := NewPublisherAt(root)
	publisher.Publish(Document{
		State:       Blocked,
		Agent:       "ompig-slim",
		SessionID:   "session-1",
		SessionFile: "/home/u/.pi/agent/sessions/s.jsonl",
		Cwd:         "/home/u/code/thing",
		Mode:        "tui",
		Detail:      "Allow write to /tmp?",
		BlockedKind: "permission",
	})

	doc := readDocument(t, root)
	if doc.State != Blocked || doc.Detail != "Allow write to /tmp?" || doc.BlockedKind != "permission" {
		t.Fatalf("document = %+v", doc)
	}
	if doc.SessionFile != "/home/u/.pi/agent/sessions/s.jsonl" || doc.Cwd != "/home/u/code/thing" || doc.Mode != "tui" {
		t.Fatalf("session fields lost: %+v", doc)
	}
}

func TestPublishWithoutADestinationDoesNothing(t *testing.T) {
	var nilPublisher *Publisher
	nilPublisher.Publish(Document{State: Working})
	if nilPublisher.Enabled() {
		t.Fatal("a nil publisher must be disabled")
	}

	disabled := NewPublisherAt("")
	disabled.Publish(Document{State: Working})
	if disabled.Enabled() {
		t.Fatal("an empty destination must be disabled")
	}
	if disabled.LastError() != nil {
		t.Fatalf("a disabled publisher recorded %v", disabled.LastError())
	}
}

func TestPublishReportsAWriteFailureWithoutFailing(t *testing.T) {
	// A directory path where a directory cannot be created: the parent is a
	// regular file.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	publisher := NewPublisherAt(filepath.Join(blocker, "state"))
	publisher.Publish(Document{State: Working})

	if publisher.LastError() == nil {
		t.Fatal("a failed write must be recorded")
	}
}

func TestRemoveDropsTheDocument(t *testing.T) {
	root := t.TempDir()
	publisher := NewPublisherAt(root)
	publisher.Publish(Document{State: Idle, Agent: "ompig-slim"})
	publisher.Remove()

	if _, err := Read(filepath.Join(root, "status.json")); err != ErrNoState {
		t.Fatalf("read after Remove = %v, want ErrNoState", err)
	}
}

func TestReadDistinguishesMissingFromCorrupt(t *testing.T) {
	dir := t.TempDir()
	if _, err := Read(filepath.Join(dir, "absent.json")); err != ErrNoState {
		t.Fatalf("missing file = %v, want ErrNoState", err)
	}

	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{"), 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	if _, err := Read(corrupt); err == nil || err == ErrNoState {
		t.Fatalf("corrupt file = %v, want a decode error", err)
	}
}

func TestDocumentJSONUsesStableFieldNames(t *testing.T) {
	data, err := json.Marshal(Document{State: Working, Agent: "ompig-slim", Detail: "why"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	for _, field := range []string{`"state":"working"`, `"agent":"ompig-slim"`, `"detail":"why"`} {
		if !strings.Contains(text, field) {
			t.Fatalf("document JSON %s lacks %s", text, field)
		}
	}
	// Empty optional fields are omitted, so a consumer sees absence rather than
	// an empty string it has to special-case.
	if strings.Contains(text, `"session_id"`) {
		t.Fatalf("an empty session_id must be omitted: %s", text)
	}
}

// TestClockIsInjectable pins that Publish stamps the injected time, which is what
// lets a caller reason about document age without a real sleep.
func TestClockIsInjectable(t *testing.T) {
	root := t.TempDir()
	publisher := NewPublisherAt(root)
	fixed := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	publisher.clock = func() time.Time { return fixed }
	publisher.Publish(Document{State: Idle, Agent: "ompig-slim"})

	if got, want := readDocument(t, root).UpdatedAt, fixed.Format(time.RFC3339Nano); got != want {
		t.Fatalf("updated_at = %q, want %q", got, want)
	}
}

// TestConcurrentSessionsKeepSeparateDocuments is the regression test for the
// defect that motivated per-process directories: with one shared file, the second
// session's publish overwrote the first, so a consumer watching a pane could see
// a rival session's state.
func TestConcurrentSessionsKeepSeparateDocuments(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PIG_HOME", root)

	firstDir := filepath.Join(root, "state", "agent", "1111")
	secondDir := filepath.Join(root, "state", "agent", "2222")
	first := NewPublisherAt(firstDir)
	second := NewPublisherAt(secondDir)
	first.Publish(Document{State: Working, Agent: "ompig-slim", SessionID: "s1", Cwd: "/a"})
	second.Publish(Document{State: Blocked, Agent: "ompig-slim", SessionID: "s2", Cwd: "/b"})

	if got := readDocument(t, firstDir); got.SessionID != "s1" || got.State != Working {
		t.Fatalf("first session's document was disturbed: %+v", got)
	}
	if got := readDocument(t, secondDir); got.SessionID != "s2" || got.State != Blocked {
		t.Fatalf("second session's document = %+v", got)
	}
}

func TestListReturnsLiveProcessDocumentsOnly(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PIG_HOME", root)
	agentRoot := filepath.Join(root, "state", "agent")

	// This process is alive; pid 0 is not a process List may report.
	live := NewPublisherAt(filepath.Join(agentRoot, strconv.Itoa(os.Getpid())))
	live.Publish(Document{State: Working, Agent: "ompig-slim", SessionID: "live"})
	dead := NewPublisherAt(filepath.Join(agentRoot, "2147483646"))
	dead.Publish(Document{State: Working, Agent: "ompig-slim", SessionID: "dead"})

	documents, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(documents) != 1 || documents[0].SessionID != "live" {
		t.Fatalf("List() = %+v, want only the live agent", documents)
	}
	// A dead writer's leftovers are cleaned up rather than reported forever.
	if _, err := os.Stat(filepath.Join(agentRoot, "2147483646")); !os.IsNotExist(err) {
		t.Fatalf("a stale document survived List: %v", err)
	}
}

func TestListOnAMachineWithNoAgents(t *testing.T) {
	t.Setenv("PIG_HOME", t.TempDir())
	documents, err := List()
	if err != nil {
		t.Fatalf("List on an empty root: %v", err)
	}
	if len(documents) != 0 {
		t.Fatalf("List() = %+v", documents)
	}
}

func TestListSkipsCorruptAndNonProcessDirectories(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PIG_HOME", root)
	agentRoot := filepath.Join(root, "state", "agent")

	if err := os.MkdirAll(filepath.Join(agentRoot, "notapid"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	corrupt := filepath.Join(agentRoot, strconv.Itoa(os.Getpid()))
	if err := os.MkdirAll(corrupt, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(corrupt, "status.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	documents, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(documents) != 0 {
		t.Fatalf("List() = %+v, want nothing readable", documents)
	}
}

func TestRemoveProcessClearsACrashedWritersLeftovers(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PIG_HOME", root)
	dir := filepath.Join(root, "state", "agent", "4242")
	publisher := NewPublisherAt(dir)
	publisher.Publish(Document{State: Working, Agent: "ompig-slim"})

	if err := RemoveProcess(4242); err != nil {
		t.Fatalf("RemoveProcess: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("directory survived RemoveProcess: %v", err)
	}
	// Removing an agent that never existed is not an error.
	if err := RemoveProcess(4242); err != nil {
		t.Fatalf("second RemoveProcess: %v", err)
	}
}

func TestLiveReportsThisProcess(t *testing.T) {
	publisher := NewPublisherAt(t.TempDir())
	publisher.Publish(Document{State: Idle, Agent: "ompig-slim"})
	publisher.mu.Lock()
	doc := *publisher.last
	publisher.mu.Unlock()

	if !Live(doc) {
		t.Fatal("the running process must report live")
	}
	doc.ProcessID = 0
	if Live(doc) {
		t.Fatal("pid 0 must not report live")
	}
}

// TestPublisherKeepsPIDAndDirectoryInAgreement pins the invariant List depends on:
// the document's pid is the directory it lives in, whether the publisher was built
// from the live process or for another one.
func TestPublisherKeepsPIDAndDirectoryInAgreement(t *testing.T) {
	for _, pid := range []int{os.Getpid(), 4242} {
		dir := filepath.Join(t.TempDir(), strconv.Itoa(pid))
		publisher := NewPublisherAt(dir)
		publisher.Publish(Document{State: Idle, Agent: "ompig-slim"})
		if got := readDocument(t, dir).ProcessID; got != pid {
			t.Fatalf("document pid = %d, want the directory's %d", got, pid)
		}
	}
}
