package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// recorder captures each Herdr call.
type recorder struct {
	mu   sync.Mutex
	args [][]string
	bins []string
}

func (r *recorder) run(_ context.Context, bin string, args []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bins = append(r.bins, bin)
	r.args = append(r.args, args)
	return nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.args)
}

// last returns the most recent call, waiting briefly for the poller.
func (r *recorder) waitFor(t *testing.T) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		if len(r.args) > 0 {
			args := r.args[len(r.args)-1]
			r.mu.Unlock()
			return args
		}
		r.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no Herdr call was made")
	return nil
}

func enabledEnv() paneEnv {
	return paneEnv{marker: "1", paneID: "w1:p1", binPath: "/opt/herdr"}
}

func flagValue(t *testing.T, args []string, flag string) string {
	t.Helper()
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatalf("flag %s missing from %v", flag, args)
	return ""
}

func indexOf(args []string, value string) int {
	for i, arg := range args {
		if arg == value {
			return i
		}
	}
	return -1
}

// writeState writes a state document into dir, as internal/agentstate does.
// writeState writes a state document. A caller that does not set `updated_at` gets
// a current timestamp, because the adapter only reports a freshly published
// document; a fixture without one would be testing the staleness path by accident.
func writeState(t *testing.T, dir string, document map[string]any) string {
	t.Helper()
	if _, ok := document["updated_at"]; !ok {
		document["updated_at"] = time.Now().Format(time.RFC3339Nano)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "status.json")
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	return path
}

func TestReportAgentCommandShape(t *testing.T) {
	rec := &recorder{}
	reporter := newReporter(enabledEnv(), rec.run)
	reporter.update(reported{state: "working", sessionFile: "/home/u/.pi/agent/sessions/s.jsonl"})

	args := rec.waitFor(t)
	if got, want := args[:5], []string{"pane", "report-agent", "w1:p1", "--source", "custom:ompig-slim"}; !equal(args[:5], want) {
		t.Fatalf("prefix = %v, want %v", got, want)
	}
	if got := flagValue(t, args, "--agent"); got != "ompig-slim" {
		t.Fatalf("agent = %q", got)
	}
	if got := flagValue(t, args, "--state"); got != "working" {
		t.Fatalf("state = %q", got)
	}
	if flagValue(t, args, "--seq") == "" {
		t.Fatal("seq must be reported so Herdr can order reports")
	}
	if idx := indexOf(args, "--message"); idx >= 0 {
		t.Fatalf("a working report must not carry a message: %v", args)
	}
	separator := indexOf(args, "--")
	if separator < 0 {
		t.Fatalf("resume argv must follow --: %v", args)
	}
	if got, want := args[separator+1:], []string{"ompig-slim", "--session", "/home/u/.pi/agent/sessions/s.jsonl"}; !equal(got, want) {
		t.Fatalf("resume argv = %v, want %v", got, want)
	}
	if got := rec.bins[0]; got != "/opt/herdr" {
		t.Fatalf("binary = %q, want HERDR_BIN_PATH", got)
	}
}

func TestBlockedReportCarriesOneLineMessage(t *testing.T) {
	rec := &recorder{}
	reporter := newReporter(enabledEnv(), rec.run)
	reporter.update(reported{state: "blocked", detail: "Allow write to /tmp?"})

	if got := flagValue(t, rec.waitFor(t), "--message"); got != "Allow write to /tmp?" {
		t.Fatalf("message = %q", got)
	}
}

func TestUnchangedStateSendsNothing(t *testing.T) {
	rec := &recorder{}
	reporter := newReporter(enabledEnv(), rec.run)
	reporter.update(reported{state: "idle"})
	reporter.update(reported{state: "idle"})
	reporter.update(reported{state: "working"})
	rec.waitFor(t)

	if got := rec.count(); got != 2 {
		t.Fatalf("sent %d reports for two distinct states", got)
	}
}

// TestFailedWorkingReportRetries pins the false-completion failure: Herdr still
// holds idle after a failed working report, even though the document is busy.
func TestFailedWorkingReportRetries(t *testing.T) {
	rec := &recorder{}
	failure := errors.New("Herdr unavailable")
	fail := false
	reporter := newReporter(enabledEnv(), func(ctx context.Context, bin string, args []string) error {
		_ = rec.run(ctx, bin, args)
		if fail {
			return failure
		}
		return nil
	})
	reporter.update(reported{state: "idle"})
	fail = true
	reporter.update(reported{state: "working"})
	fail = false
	reporter.update(reported{state: "working"})
	reporter.update(reported{state: "working"})
	if got := rec.count(); got != 3 {
		t.Fatalf("got %d calls, want idle, failed working, successful working", got)
	}
	if got := flagValue(t, rec.waitFor(t), "--state"); got != "working" {
		t.Fatalf("last state = %q, want working", got)
	}
	if first, retry := flagValue(t, rec.args[1], "--seq"), flagValue(t, rec.args[2], "--seq"); first >= retry {
		t.Fatalf("retry sequence did not advance: %s then %s", first, retry)
	}
}

// TestRunCLIReturnsFailure ensures process errors reach retry logic rather than
// being swallowed at the transport boundary.
func TestRunCLIReturnsFailure(t *testing.T) {
	if err := runCLI(context.Background(), filepath.Join(t.TempDir(), "missing-herdr"), nil); err == nil {
		t.Fatal("a missing Herdr executable must return an error")
	}
}

func TestSequenceIncreases(t *testing.T) {
	rec := &recorder{}
	reporter := newReporter(enabledEnv(), rec.run)
	reporter.update(reported{state: "idle"})
	reporter.update(reported{state: "working"})
	rec.waitFor(t)

	rec.mu.Lock()
	args := append([][]string(nil), rec.args...)
	rec.mu.Unlock()
	if len(args) != 2 {
		t.Fatalf("got %d reports", len(args))
	}
	first, second := flagValue(t, args[0], "--seq"), flagValue(t, args[1], "--seq")
	if !(first < second) {
		t.Fatalf("seq did not increase: %s then %s", first, second)
	}
}

func TestFinishReportsOnceAndStops(t *testing.T) {
	rec := &recorder{}
	reporter := newReporter(enabledEnv(), rec.run)
	reporter.update(reported{state: "working"})
	reporter.finish(reported{state: "idle"})
	reporter.update(reported{state: "working"})

	if got := rec.count(); got != 2 {
		t.Fatalf("sent %d reports, want the working state and one finish", got)
	}
	if got := flagValue(t, rec.waitFor(t), "--state"); got != "idle" {
		t.Fatalf("last state = %q, want idle", got)
	}
}

func TestDisabledEnvironmentSendsNothing(t *testing.T) {
	rec := &recorder{}
	for _, env := range []paneEnv{
		{},
		{marker: "0", paneID: "w1:p1", binPath: "/opt/herdr"},
		{marker: "1", paneID: "", binPath: "/opt/herdr"},
		{marker: "1", paneID: "w1:p1", binPath: ""},
	} {
		reporter := newReporter(env, rec.run)
		reporter.update(reported{state: "working"})
		reporter.finish(reported{state: "idle"})
	}
	if got := rec.count(); got != 0 {
		t.Fatalf("sent %d reports outside a Herdr pane", got)
	}
}

func TestReadReportedMapsPublishedStates(t *testing.T) {
	dir := t.TempDir()
	for _, state := range []string{"idle", "working", "blocked"} {
		path := writeState(t, dir, map[string]any{"state": state, "session_file": "/tmp/s.jsonl"})
		got, ok := readReported(path)
		if !ok || got.state != state {
			t.Fatalf("readReported(%q) = %+v, %v", state, got, ok)
		}
		if got.sessionFile != "/tmp/s.jsonl" {
			t.Fatalf("session file lost: %+v", got)
		}
	}
}

func TestReadReportedIgnoresExitedAndUnknownStates(t *testing.T) {
	dir := t.TempDir()
	for _, state := range []string{"exited", "", "bogus"} {
		path := writeState(t, dir, map[string]any{"state": state})
		if got, ok := readReported(path); ok {
			t.Fatalf("readReported(%q) = %+v, want no report", state, got)
		}
	}
}

func TestReadReportedKeepsAMessageOnlyWhenBlocked(t *testing.T) {
	dir := t.TempDir()
	path := writeState(t, dir, map[string]any{"state": "idle", "detail": "stale dialog title"})
	if got, ok := readReported(path); !ok || got.detail != "" {
		t.Fatalf("an idle document kept a message: %+v, %v", got, ok)
	}

	path = writeState(t, dir, map[string]any{"state": "blocked", "detail": "Allow write?"})
	if got, ok := readReported(path); !ok || got.detail != "Allow write?" {
		t.Fatalf("a blocked document lost its message: %+v, %v", got, ok)
	}
}

func TestReadReportedToleratesAMissingOrCorruptDocument(t *testing.T) {
	dir := t.TempDir()
	if _, ok := readReported(filepath.Join(dir, "absent.json")); ok {
		t.Fatal("a missing document must report nothing")
	}

	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, ok := readReported(corrupt); ok {
		t.Fatal("a corrupt document must report nothing")
	}
}

// TestReadReportedDropsAStaleDocument pins the reader's half of the heartbeat
// contract.
//
// Herdr never expires a reported state, so a document that stopped being refreshed
// would keep a pane looking busy forever: the observed symptom was a session whose
// document froze at `working` on its first turn. A reader therefore reports nothing
// for a document older than the harness's own heartbeat window, and keeps reporting
// inside it, so one missed refresh does not blank a healthy agent.
func TestReadReportedDropsAStaleDocument(t *testing.T) {
	dir := t.TempDir()
	published := time.Now()
	path := writeState(t, dir, map[string]any{
		"state":      "working",
		"updated_at": published.Format(time.RFC3339Nano),
	})

	if got, ok := readReportedAt(path, published.Add(staleAfter)); !ok || got.state != "working" {
		t.Fatalf("a document at the staleness edge was dropped: %+v, %v", got, ok)
	}
	if _, ok := readReportedAt(path, published.Add(staleAfter+time.Second)); ok {
		t.Fatal("a stale document must report nothing")
	}

	// A document with no readable timestamp has no claim to being current.
	for name, content := range map[string]string{
		"absent timestamp":  `{"state":"working"}`,
		"unparseable value": `{"state":"working","updated_at":"yesterday"}`,
	} {
		path := filepath.Join(t.TempDir(), "status.json")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, ok := readReportedAt(path, published); ok {
			t.Fatalf("%s: a document with no usable timestamp must report nothing", name)
		}
	}
}

func TestResumeArgsRejectWhatHerdrRejects(t *testing.T) {
	if got, want := resumeArgs("/tmp/s.jsonl"), []string{"ompig-slim", "--session", "/tmp/s.jsonl"}; !equal(got, want) {
		t.Fatalf("resumeArgs = %v, want %v", got, want)
	}
	for _, path := range []string{"", "relative.jsonl", "nested/s.jsonl"} {
		if got := resumeArgs(path); got != nil {
			t.Fatalf("resumeArgs(%q) = %v, want nil", path, got)
		}
	}
	// A path Herdr cannot quote still sends the state without the resume.
	if got := resumeArgs("/tmp/it's.jsonl"); got != nil {
		t.Fatalf("resumeArgs kept an unquotable path: %v", got)
	}
}

func TestValidResumeArgvMatchesHerdrContract(t *testing.T) {
	for _, valid := range [][]string{{"ompig-slim", "--session", "/tmp/s.jsonl"}, {resumeCommand}} {
		if !validResumeArgv(valid) {
			t.Fatalf("validResumeArgv(%v) rejected a usable command", valid)
		}
	}
	tooMany := []string{resumeCommand}
	for len(tooMany) <= maxResumeArgs {
		tooMany = append(tooMany, "x")
	}
	for _, invalid := range [][]string{
		nil,
		{},
		{"/usr/local/bin/ompig-slim"},
		{"ompig-slim --session"},
		{"-s"},
		{"ompig-slim", "it's"},
		{"ompig-slim", "a\nb"},
		{resumeCommand, strings.Repeat("x", maxResumeArgvBytes)},
		tooMany,
	} {
		if validResumeArgv(invalid) {
			t.Fatalf("validResumeArgv(%v) accepted an unusable command", invalid)
		}
	}
}

func TestIsAbsolutePathAcceptsBothPlatformForms(t *testing.T) {
	for _, path := range []string{"/tmp/s.jsonl", `\\server\share\s.jsonl`, `C:\s.jsonl`, `c:/s.jsonl`} {
		if !isAbsolutePath(path) {
			t.Fatalf("isAbsolutePath(%q) = false", path)
		}
	}
	for _, path := range []string{"", "s.jsonl", "1:/s.jsonl", "C:", "nested/s.jsonl"} {
		if isAbsolutePath(path) {
			t.Fatalf("isAbsolutePath(%q) = true", path)
		}
	}
}

func TestSessionForwardsPublishedStateToHerdr(t *testing.T) {
	// The document lives in one directory per process, so the watcher's root is
	// the agents directory and the document is one level below it.
	root := t.TempDir()
	dir := filepath.Join(root, "4242")
	rec := &recorder{}
	session := newSession(enabledEnv(), func() (string, bool) { return root, true }, rec.run)

	startWatching(t, session, root, "session-1")
	writeState(t, dir, map[string]any{"state": "working", "session_file": "/tmp/s.jsonl", "session_id": "session-1"})
	if got := flagValue(t, rec.waitFor(t), "--state"); got != "working" {
		t.Fatalf("state = %q, want working", got)
	}

	writeState(t, dir, map[string]any{"state": "blocked", "detail": "Approve?", "session_id": "session-1"})
	// The poller must pick the change up on its own.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := flagValue(t, rec.waitFor(t), "--state"); got == "blocked" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := flagValue(t, rec.waitFor(t), "--state"); got != "blocked" {
		t.Fatalf("state = %q, want blocked", got)
	}

	session.stop(sdk.Context{}, true)
	if got := flagValue(t, rec.waitFor(t), "--state"); got != "idle" {
		t.Fatalf("final state = %q, want idle", got)
	}
}

// startWatching runs the poller for sessionID, standing in for session_start: a
// Context cannot be constructed outside a host connection, so the test drives the
// two things session_start does — record the id and start the goroutine.
func startWatching(t *testing.T, session *session, dir, sessionID string) {
	t.Helper()
	session.mu.Lock()
	session.sessionID = sessionID
	watchCtx, cancel := context.WithCancel(context.Background())
	session.cancel = cancel
	session.mu.Unlock()
	go session.watch(watchCtx, sessionID, session.reporter)
}

func TestStoppingTwiceReportsOnce(t *testing.T) {
	root := t.TempDir()
	rec := &recorder{}
	session := newSession(enabledEnv(), func() (string, bool) { return root, true }, rec.run)

	startWatching(t, session, root, "session-1")
	session.stop(sdk.Context{}, true)
	session.stop(sdk.Context{}, true)
	if got := rec.count(); got != 1 {
		t.Fatalf("sent %d reports for two stops", got)
	}
}

// TestSessionReplacementDoesNotReportFinished ensures session_shutdown for a
// live process does not make Herdr report the agent as finished.
func TestSessionReplacementDoesNotReportFinished(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "4242")
	rec := &recorder{}
	session := newSession(enabledEnv(), func() (string, bool) { return root, true }, rec.run)

	startWatching(t, session, root, "session-1")
	writeState(t, dir, map[string]any{"state": "working", "session_id": "session-1"})
	if got := flagValue(t, rec.waitFor(t), "--state"); got != "working" {
		t.Fatalf("state = %q, want working", got)
	}

	// /clear, /reload, resume, and fork replace a session without ending the
	// process. The adapter must not report idle during that handoff.
	session.stop(sdk.Context{}, false)
	if got := rec.count(); got != 1 {
		t.Fatalf("replacement shutdown sent %d reports, want no extra report", got)
	}

	startWatching(t, session, root, "session-2")
	writeState(t, dir, map[string]any{"state": "blocked", "session_id": "session-2"})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if rec.count() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := flagValue(t, rec.waitFor(t), "--state"); got != "blocked" {
		t.Fatalf("replacement state = %q, want blocked", got)
	}
}

func TestAgentRootFollowsPigHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PIG_HOME", root)
	path, ok := agentRoot()
	if !ok {
		t.Fatal("agentRoot reported no root")
	}
	if want := filepath.Join(root, "state", "agent"); path != want {
		t.Fatalf("agentRoot = %q, want %q", path, want)
	}
}

// TestFindDocumentMatchesTheSessionNotTheProcess pins why the extension matches on
// session id: PiG runs it in its own process, so its pid never names the agent's
// document. Two agents publishing at once must not be confused.
func TestFindDocumentMatchesTheSessionNotTheProcess(t *testing.T) {
	root := t.TempDir()
	firstDir := filepath.Join(root, "1111")
	secondDir := filepath.Join(root, "2222")
	writeState(t, firstDir, map[string]any{"state": "working", "session_id": "other"})
	writeState(t, secondDir, map[string]any{"state": "blocked", "session_id": "mine"})

	path, ok := findDocument(root, "mine")
	if !ok {
		t.Fatal("findDocument did not find the session's own document")
	}
	if want := filepath.Join(secondDir, "status.json"); path != want {
		t.Fatalf("findDocument = %q, want %q", path, want)
	}
	if _, ok := findDocument(root, "absent"); ok {
		t.Fatal("findDocument matched a session that has no document")
	}
	if _, ok := findDocument(root, ""); ok {
		t.Fatal("findDocument matched an empty session id")
	}
}

func equal(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
