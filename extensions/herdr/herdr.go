// Package herdr adapts ompig-slim's published agent state to Herdr.
//
// It is an ordinary PiG extension, not part of the binary: ompig-slim exposes
// what the agent is doing as a JSON document
// (<PIG_HOME>/state/agent/status.json, see internal/agentstate) and this
// extension translates that document into `herdr pane report-agent` calls. Any
// other consumer — a tmux status line, a notifier, a dashboard — reads the same
// document and needs no change here, and a user who does not run Herdr pays
// nothing for this file's existence.
//
// Why Herdr needs an adapter at all: Herdr decides which agent owns a pane from
// the process name of the pane's foreground job, against a closed alias table
// (`pi`, `omp`, `claude`, ...). `ompig-slim` is not in that table, so a Herdr
// pane running it looks like a plain shell. Herdr's documented integration point
// for an agent it does not ship support for is `pane report-agent`: a reported
// state overrides screen detection and drives `herdr agent wait`, notifications,
// and the tab and workspace rollups.
//
// The extension is a pure adapter. It derives no state of its own, so it cannot
// disagree with what ompig-slim publishes, and a bug here can only misreport a
// state that is already visible on disk.
package herdr

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

const (
	// source names this integration to Herdr. Herdr reserves the `herdr:` prefix
	// for the integrations it ships and requires every other reporter to keep a
	// stable, unique source outside it.
	source = "custom:ompig-slim"
	// agentLabel is the name Herdr's sidebar, `herdr agent list`, and the
	// `herdr agent wait` and `agent prompt` target syntax show for this program.
	agentLabel = "ompig-slim"
	// resumeCommand opens a session in a restored pane. Herdr types the command
	// into a fresh shell and rejects a path, so this must be a bare command name
	// that resolves on PATH; the installer's placement decides what it finds.
	resumeCommand = "ompig-slim"

	// pollInterval is how often the state document is re-read. The document is a
	// few hundred bytes and a publish only happens when the state changes, so
	// this is cheap; a faster interval would only add process wakeups without
	// making a sidebar feel more responsive.
	pollInterval = 250 * time.Millisecond
	// sendTimeout bounds one Herdr call. Herdr answers in milliseconds; anything
	// slower means the server is gone, and a late report cannot describe the
	// current state anyway.
	sendTimeout = 2 * time.Second

	// Herdr's resume-argv limits. A report carrying a resume command that breaks
	// any of them is rejected whole, so the command is checked here and left out
	// rather than costing the state report.
	maxResumeArgs      = 64
	maxResumeArgvBytes = 8 * 1024
)

// paneEnv is the pane environment Herdr exports to every process a pane starts.
type paneEnv struct {
	// marker is HERDR_ENV, "1" inside a Herdr pane.
	marker string
	// paneID is HERDR_PANE_ID, the pane this process runs in.
	paneID string
	// binPath is HERDR_BIN_PATH, the Herdr executable that owns the pane.
	binPath string
}

func paneEnvFromOS() paneEnv {
	return paneEnv{
		marker:  os.Getenv("HERDR_ENV"),
		paneID:  os.Getenv("HERDR_PANE_ID"),
		binPath: os.Getenv("HERDR_BIN_PATH"),
	}
}

// enabled reports whether this process runs inside a Herdr pane that can accept
// reports, which is every pane.
func (e paneEnv) enabled() bool {
	return e.marker == "1" && e.paneID != "" && e.binPath != ""
}

// reported is the part of the state document this adapter forwards.
type reported struct {
	state       string
	detail      string
	blockedKind string
	sessionFile string
}

// run executes one Herdr CLI call. Tests replace it to observe argv.
type runFunc func(ctx context.Context, bin string, args []string)

// reporter forwards state to one Herdr pane.
//
// It keeps the newest state and writes it as fast as Herdr answers, so a slow
// Herdr cannot make the sidebar lag behind the agent: intermediate states are
// dropped rather than queued.
type reporter struct {
	env paneEnv
	run runFunc

	mu sync.Mutex
	// seq orders reports within one source. Herdr drops a report whose seq is
	// not higher than the last one it accepted from that source, so it must keep
	// rising across agent restarts in the same pane; the wall clock start
	// guarantees that.
	seq uint64
	// last is the state last sent, so an unchanged document sends nothing.
	last *reported
	// sentExited records that the terminal state was reported, after which
	// nothing more is sent for this pane.
	sentExited bool
}

func newReporter(env paneEnv, run runFunc) *reporter {
	return &reporter{env: env, run: run, seq: uint64(time.Now().UnixNano())}
}

// update sends state if it differs from the last one sent.
func (r *reporter) update(state reported) {
	if !r.env.enabled() {
		return
	}
	r.mu.Lock()
	if r.sentExited {
		r.mu.Unlock()
		return
	}
	if r.last != nil && *r.last == state {
		r.mu.Unlock()
		return
	}
	stored := state
	r.last = &stored
	r.seq++
	args := r.argv(state, r.seq)
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	r.run(ctx, r.env.binPath, args)
}

// finish reports the terminal state and stops further reporting.
func (r *reporter) finish(state reported) {
	if !r.env.enabled() {
		return
	}
	r.mu.Lock()
	if r.sentExited {
		r.mu.Unlock()
		return
	}
	r.sentExited = true
	r.seq++
	args := r.argv(state, r.seq)
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	r.run(ctx, r.env.binPath, args)
}

// argv is the `herdr pane report-agent` command line for one state. The resume
// command follows a `--` terminator because Herdr reads it as a positional list
// after its own flags.
func (r *reporter) argv(state reported, seq uint64) []string {
	args := []string{
		"pane", "report-agent", r.env.paneID,
		"--source", source,
		"--agent", agentLabel,
		"--state", state.state,
		"--seq", strconv.FormatUint(seq, 10),
	}
	if state.detail != "" {
		args = append(args, "--message", state.detail)
	}
	if resume := resumeArgs(state.sessionFile); len(resume) > 0 {
		args = append(args, "--")
		args = append(args, resume...)
	}
	return args
}

// runCLI executes one Herdr call. Herdr writes a JSON line and exits non-zero on
// a rejected report; neither is actionable here, so both streams are discarded.
func runCLI(ctx context.Context, bin string, args []string) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	_ = cmd.Run()
}

// resumeArgs is the command that reopens the session in path, or nil when the
// session has no file or Herdr would reject the command.
//
// Herdr's restore types the command into a fresh shell in the pane's directory,
// so the session is named by absolute path rather than by an ID lookup that
// depends on the pane's original cwd.
func resumeArgs(path string) []string {
	if path == "" || !isAbsolutePath(path) {
		return nil
	}
	argv := []string{resumeCommand, "--session", path}
	if !validResumeArgv(argv) {
		return nil
	}
	return argv
}

// isAbsolutePath reports whether path is absolute on any platform Herdr runs on.
// filepath.IsAbs is not used because the state document may have been written by
// a Windows build and read by a Unix one through a shared profile.
func isAbsolutePath(path string) bool {
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\\`) {
		return true
	}
	return len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') &&
		((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z'))
}

// validResumeArgv applies Herdr's resume-argv contract: a plain command name, no
// control characters or apostrophes (Herdr quotes POSIX-style, and PowerShell
// reads an apostrophe differently), at most 64 arguments and 8 KiB in total.
func validResumeArgv(argv []string) bool {
	if len(argv) == 0 || len(argv) > maxResumeArgs {
		return false
	}
	total := 0
	for _, arg := range argv {
		total += len(arg)
		if strings.ContainsRune(arg, '\'') || strings.ContainsFunc(arg, isControlRune) {
			return false
		}
	}
	if total > maxResumeArgvBytes {
		return false
	}
	command := argv[0]
	if command == "" || strings.HasPrefix(command, "-") {
		return false
	}
	for _, r := range command {
		plain := r == '_' || r == '-' || r == '.' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !plain {
			return false
		}
	}
	return true
}

func isControlRune(r rune) bool { return r < ' ' || r == '\u007f' }

// agentRoot resolves the directory holding one document per running agent: PIG_HOME
// when set, else ~/.pi.
//
// The harness resolves the same layout; both spell it out rather than sharing a
// constant because this extension is built against PiG's staged SDK, not against
// the harness's module.
func agentRoot() (string, bool) {
	root := strings.TrimSpace(os.Getenv("PIG_HOME"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		root = filepath.Join(home, ".pi")
	}
	return filepath.Join(root, "state", "agent"), true
}

// findDocument returns the document of the session running as sessionID.
//
// The extension cannot use its own process id: PiG runs it in a separate process
// from the agent, so the document keyed by its pid belongs to nobody. It knows the
// session identifier, which every document carries, so it matches on that. A
// session replacement changes the identifier, and the next poll finds the new
// document without any explicit re-subscription.
func findDocument(root, sessionID string) (string, bool) {
	if sessionID == "" {
		return "", false
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "status.json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc statusDocument
		if err := json.Unmarshal(data, &doc); err != nil {
			continue
		}
		if doc.SessionID == sessionID {
			return path, true
		}
	}
	return "", false
}

// Extension is the factory PiG loads.
//
// It registers no tool, command, or flag: it exists only to watch the state
// document while a session runs, so it cannot shadow a user's extension or
// appear in the tool list.
func Extension() *sdk.Extension {
	ext := sdk.New("herdr")
	env := paneEnvFromOS()
	if !env.enabled() {
		// Outside Herdr there is nothing to do, and returning a bare extension
		// keeps the startup Extensions list honest about that.
		return ext
	}
	session := newSession(env, agentRoot, runCLI)

	ext.OnSessionStart(func(ctx sdk.Context, _ map[string]any) (any, error) {
		session.start(ctx)
		return nil, nil
	})
	ext.OnSessionShutdown(func(ctx sdk.Context, _ map[string]any) (any, error) {
		session.stop(ctx)
		return nil, nil
	})
	return ext
}

// session owns the poller for one session.
type session struct {
	env       paneEnv
	agentRoot func() (string, bool)
	reporter  *reporter
	// sessionID is the session this extension reports for. It is read at
	// session_start, because a process id is no guide: the extension runs in its
	// own process.
	sessionID string

	mu      sync.Mutex
	cancel  context.CancelFunc
	stopped bool
}

func newSession(env paneEnv, root func() (string, bool), run runFunc) *session {
	return &session{env: env, agentRoot: root, reporter: newReporter(env, run)}
}

// start begins watching the state document. A session that is already watching
// keeps its poller, so a repeated session_start (a reload) does not leak one.
func (s *session) start(ctx sdk.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil || s.stopped {
		return
	}
	if id, err := ctx.GetSessionID(); err == nil {
		s.sessionID = id
	}
	watchCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go s.watch(watchCtx, s.sessionID, s.reporter)
}

// stop ends the poller and reports the terminal state, so a consumer stops
// waiting instead of trusting the last `working` forever.
func (s *session) stop(_ sdk.Context) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	stopPolling := s.cancel
	s.mu.Unlock()
	if stopPolling != nil {
		stopPolling()
	}
	// Report idle rather than exiting: a Herdr pane whose agent quit is back at
	// its shell prompt, and Herdr clears the pane on its own from there.
	s.reporter.finish(reported{state: "idle"})
}

// watch polls this session's document until ctx ends, forwarding each change.
func (s *session) watch(ctx context.Context, sessionID string, reporter *reporter) {
	root, ok := s.agentRoot()
	if !ok {
		return
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		path, found := findDocument(root, sessionID)
		if !found {
			// The document may not exist yet (a fresh session publishes at its
			// first event) or the session may have been replaced.
			continue
		}
		state, ok := readReported(path)
		if !ok {
			// No document yet, or an unreadable one. Reporting nothing is
			// correct: the previous state is still the last thing that was true,
			// and Herdr clears the pane itself once the shell prompt returns.
			continue
		}
		reporter.update(state)
	}
}
