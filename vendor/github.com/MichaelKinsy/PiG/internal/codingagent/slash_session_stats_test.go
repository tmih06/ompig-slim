package codingagent

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// traceTrack returns the 40 track cells of the trace row that starts with
// the given lane name, so tests read where a lane was busy.
func traceTrack(t *testing.T, out, name string) []rune {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimLeft(line, " ●✓✗■◦⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		if !strings.HasPrefix(trimmed, name) {
			continue
		}
		cells := []rune(strings.TrimLeft(strings.TrimPrefix(trimmed, name), " "))
		if len(cells) >= sessionTraceWidth && strings.ContainsAny(string(cells[:sessionTraceWidth]), "·█▌▐") {
			return cells[:sessionTraceWidth]
		}
	}
	t.Fatalf("no trace row for %q in:\n%s", name, out)
	return nil
}

func busyCells(track []rune) []int {
	var busy []int
	for i, c := range track {
		if c != '·' {
			busy = append(busy, i)
		}
	}
	return busy
}

func TestSessionStatsLayout(t *testing.T) {
	session := NewSession("stats-test", t.TempDir())
	now := time.Now().UnixMilli()
	at := func(minutes int64) int64 { return now - minutes*60_000 }
	add := func(message agent.AgentMessage) {
		t.Helper()
		if _, err := session.AppendMessage(message); err != nil {
			t.Fatal(err)
		}
	}
	user := func(minutes int64) {
		add(agent.AgentMessage{User: &agent.UserMessage{Role: "user", Content: ai.UserText("hi"), Timestamp: at(minutes)}})
	}
	assistant := func(minutes int64, provider, model string, in int) {
		add(agent.AgentMessage{Assistant: &agent.AssistantMessage{
			Role: "assistant", Provider: provider, ModelID: model, Timestamp: at(minutes),
			Usage: &ai.Usage{Input: in, Output: in / 10, Cost: ai.UsageCost{Total: float64(in) / 1e6}},
		}})
	}
	toolResult := func(minutes int64) {
		add(agent.AgentMessage{ToolResult: &agent.ToolResultMessage{Role: "toolResult", ToolName: "bash", Timestamp: at(minutes)}})
	}
	// Model X works for the first ten minutes, goes idle, and works again
	// near the end; model Y runs briefly in between. The user is away from
	// minute 28 to minute 10, which must not be drawn as work.
	user(40)
	assistant(40, "anthropic", "x-model", 9000)
	toolResult(35)
	assistant(35, "anthropic", "x-model", 9000)
	toolResult(30)
	assistant(30, "openai", "y-model", 3000)
	toolResult(28)
	user(10)
	assistant(10, "anthropic", "x-model", 9000)
	toolResult(5)
	assistant(5, "anthropic", "x-model", 9000)

	records := []subagents.Record{
		{ID: "a1", DisplayName: "scout", Status: "completed", StartedAt: at(38), CompletedAt: at(30), Tokens: 12000, Cost: 0.004},
		{ID: "a2", DisplayName: "worker", Status: "completed", StartedAt: at(36), CompletedAt: at(29), Tokens: 8000},
		{ID: "a3", DisplayName: "late", Status: "running", StartedAt: at(9), Tokens: 500},
	}
	sc := &SlashContext{
		CurrentSession:   func() *Session { return session },
		SessionSubagents: func() []subagents.Record { return records },
	}
	out := widthx.StripTerminalSequences(sessionInfoRenderer(sc)(sessionTraceWidth))

	for _, want := range []string{
		"Session stats", "stats-test",
		"Messages", "Input tokens", "Output tokens", "Started",
		"Tokens by model", "anthropic/x-model", "openai/y-model",
		"Subagents", "scout", "worker", "late", "3 agents",
		"Cache Warming", "Cost",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}

	x := busyCells(traceTrack(t, out, "anthropic/x-model"))
	y := busyCells(traceTrack(t, out, "openai/y-model"))
	scout := busyCells(traceTrack(t, out, "scout"))
	worker := busyCells(traceTrack(t, out, "worker"))
	late := busyCells(traceTrack(t, out, "late"))
	if len(x) == 0 || len(y) == 0 || len(scout) == 0 || len(worker) == 0 || len(late) == 0 {
		t.Fatalf("a lane has no bar:\n%s", out)
	}

	// The trace is continuous: the user's away time (minutes 28 to 10) is cut
	// from the axis, so some lane is busy in every column.
	lanes := [][]int{x, y, scout, worker, late}
	for col := 0; col < sessionTraceWidth; col++ {
		busy := false
		for _, lane := range lanes {
			busy = busy || slices.Contains(lane, col)
		}
		if !busy {
			t.Errorf("column %d is idle in every lane; the trace is not continuous:\n%s", col, out)
			break
		}
	}
	for _, want := range []string{"idle cut", "┆"} {
		if !strings.Contains(out, want) {
			t.Errorf("report does not mark the cut idle time with %q:\n%s", want, out)
		}
	}
	// X works first, steps aside while Y and the subagents run, then works again.
	if x[0] != 0 || y[0] < x[len(x)/2] {
		t.Errorf("model X does not start the trace: x %v y %v", x, y)
	}
	gap := 0
	for i := 1; i < len(x); i++ {
		gap = max(gap, x[i]-x[i-1]-1)
	}
	if gap < 2 {
		t.Errorf("model X should show where Y took over, longest gap %d cells: %v", gap, x)
	}
	// Subagents that overlapped in time share columns; a later one does not.
	shared := false
	for _, a := range scout {
		if slices.Contains(worker, a) {
			shared = true
		}
	}
	if !shared {
		t.Errorf("concurrent subagents share no column: scout %v worker %v", scout, worker)
	}
	if late[0] <= scout[len(scout)-1] {
		t.Errorf("late subagent should start after scout ends: scout %v late %v", scout, late)
	}
	// A running subagent is drawn up to now, the right edge of the trace.
	if late[len(late)-1] != sessionTraceWidth-1 {
		t.Errorf("running subagent should reach the right edge: %v", late)
	}
}

func TestSessionSpansSurviveReload(t *testing.T) {
	manager := NewSessionManagerWithDir(t.TempDir(), t.TempDir())
	session, err := manager.Create("persist", "")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UnixMilli() - 3_600_000
	messages := []agent.AgentMessage{
		{User: &agent.UserMessage{Role: "user", Content: ai.UserText("go"), Timestamp: base}},
		{Assistant: &agent.AssistantMessage{Role: "assistant", Provider: "p", ModelID: "m", Timestamp: base + 1_000, Usage: &ai.Usage{Input: 10}}},
		{ToolResult: &agent.ToolResultMessage{Role: "toolResult", ToolName: "bash", Timestamp: base + 61_000}},
		{Assistant: &agent.AssistantMessage{Role: "assistant", Provider: "p", ModelID: "m", Timestamp: base + 62_000, Usage: &ai.Usage{Input: 10}}},
		{User: &agent.UserMessage{Role: "user", Content: ai.UserText("more"), Timestamp: base + 900_000}},
	}
	for _, message := range messages {
		if _, err := session.AppendMessage(message); err != nil {
			t.Fatal(err)
		}
	}
	live := session.Accounting().UsageBreakdown
	// The first turn runs to the next request of the same model, so both
	// turns are one span; it ends where the last turn began, and the 14 idle
	// minutes before the user's next message are not work.
	if want := (SessionSpan{base + 1_000, base + 62_000}); len(live) != 1 || !slices.Equal(live[0].Spans, []SessionSpan{want}) {
		t.Fatalf("live spans = %+v, want [%+v]", live, want)
	}
	loaded, err := manager.Load(session.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Accounting().UsageBreakdown; len(got) != 1 || !slices.Equal(got[0].Spans, live[0].Spans) {
		t.Errorf("reloaded spans = %+v, live %+v", got[0].Spans, live[0].Spans)
	}
}

func TestSessionTraceTrack(t *testing.T) {
	plain := func(slot int, s string) string { return s }
	track := func(lane []SessionSpan, all ...SessionSpan) string {
		return sessionTraceTrack(sessionLane{spans: lane}, newTraceAxis(all, sessionTraceWidth), plain, func(s string) string { return s })
	}
	if got := track(nil, SessionSpan{1000, 1800}); got != strings.Repeat("·", sessionTraceWidth) {
		t.Errorf("untimed lane = %q", got)
	}
	full := []SessionSpan{{1000, 1800}}
	if got := track(full, full...); got != strings.Repeat("█", sessionTraceWidth) {
		t.Errorf("full span = %q", got)
	}
	// 800ms over 80 slices: 10ms each, so an instant fills half a cell.
	if got := []rune(track([]SessionSpan{{1000, 1000}}, full...)); got[0] != '▌' {
		t.Errorf("instant at start = %q", string(got))
	}
	if got := []rune(track([]SessionSpan{{1800, 1800}}, full...)); got[sessionTraceWidth-1] != '▐' {
		t.Errorf("instant at end = %q", string(got))
	}
	// A lane that stops and resumes shows its own gap while another lane works.
	other := SessionSpan{1100, 1700}
	mine := []SessionSpan{{1000, 1100}, {1700, 1800}}
	got := []rune(track(mine, append(mine, other)...))
	if got[0] != '█' || got[sessionTraceWidth/2] != '·' || got[sessionTraceWidth-1] != '█' {
		t.Errorf("lane with a gap = %q", string(got))
	}
	// Idle time no lane worked through is cut: the two runs sit side by side.
	runs := []SessionSpan{{1000, 1100}, {9_000_000, 9_000_100}}
	if got := []rune(track(runs[1:], runs...)); got[0] != '·' || got[sessionTraceWidth-1] != '█' || strings.Count(string(got), "█") != sessionTraceWidth/2 {
		t.Errorf("idle gap was not cut: %q", string(got))
	}
	// A zero-length session puts every instant in the first slice.
	one := []SessionSpan{{5, 5}}
	if got := []rune(track(one, one...)); got[0] != '▌' {
		t.Errorf("single instant = %q", string(got))
	}
}

func TestTraceAxisOffset(t *testing.T) {
	const min = int64(60_000)
	axis := newTraceAxis([]SessionSpan{{1000, 1000 + 10*min}, {5000 + 40*min, 5000 + 50*min}}, sessionTraceWidth)
	if axis.active != 20*min {
		t.Fatalf("active = %d, want %d", axis.active, 20*min)
	}
	for at, want := range map[int64]int64{
		1000:          0,
		1000 + 5*min:  5 * min,
		1000 + 20*min: 10 * min, // inside the cut gap: end of the first run
		5000 + 40*min: 10 * min,
		5000 + 45*min: 15 * min,
		5000 + 90*min: 20 * min,
	} {
		if got := axis.offset(at); got != want {
			t.Errorf("offset(%d) = %d, want %d", at, got, want)
		}
	}
	if axis.idle != 30*min+4000 {
		t.Errorf("idle = %d", axis.idle)
	}
}

func TestSessionHandlerOpensViewOnlyWhenOffered(t *testing.T) {
	session := NewSession("handler-test", t.TempDir())
	var transcript []string
	sc := &SlashContext{
		CurrentSession: func() *Session { return session },
		AppendText:     func(text string) { transcript = append(transcript, text) },
	}
	if err := sessionHandler(sc); err != nil {
		t.Fatal(err)
	}
	if len(transcript) != 1 || !strings.Contains(widthx.StripTerminalSequences(transcript[0]), "handler-test") {
		t.Fatalf("without a view the report stays in the transcript: %q", transcript)
	}

	transcript, opened := nil, 0
	sc.ShowSessionStats = func(snapshot func() sessionReport) {
		opened++
		if out := widthx.StripTerminalSequences(snapshot()(sessionTraceWidth)); !strings.Contains(out, "handler-test") {
			t.Errorf("snapshot lacks the session: %q", out)
		}
	}
	if err := sessionHandler(sc); err != nil {
		t.Fatal(err)
	}
	if opened != 1 || len(transcript) != 0 {
		t.Fatalf("view opened %d times, transcript %q; want one view and an untouched transcript", opened, transcript)
	}
}

func TestSessionStatsViewFramesScrollsAndCloses(t *testing.T) {
	var closed, renders, snapshots, builds int
	const height = 10
	text := func(traceWidth int) string {
		builds++
		var rows []string
		for i := range 30 {
			rows = append(rows, "row "+strings.Repeat("x", i)+" w"+strconv.Itoa(traceWidth))
		}
		return strings.Join(rows, "\n")
	}
	view := &sessionStatsView{
		snapshot:      func() sessionReport { snapshots++; return text },
		report:        text,
		height:        func() int { return height },
		close:         func() { closed++ },
		requestRender: func() { renders++ },
	}
	plain := func(width int) []string {
		lines := view.Render(width)
		for i, line := range lines {
			lines[i] = widthx.StripTerminalSequences(line)
			if got := widthx.VisibleWidth(line); got != width {
				t.Fatalf("line %d is %d cells wide, want %d: %q", i, got, width, lines[i])
			}
		}
		return lines
	}

	lines := plain(60)
	if len(lines) != height {
		t.Fatalf("view is %d rows, want the %d of the screen", len(lines), height)
	}
	if !strings.HasPrefix(lines[1], "│ row  w") || !strings.Contains(lines[height-2], "6/30") {
		t.Errorf("first page wrong:\n%s", strings.Join(lines, "\n"))
	}

	view.HandleInput("\x1b[6~") // page down
	lines = plain(60)
	if !strings.Contains(lines[1], "row xxxxxx") {
		t.Errorf("page down did not move a page:\n%s", strings.Join(lines, "\n"))
	}
	view.HandleInput("\x1b[F") // end
	lines = plain(60)
	if !strings.Contains(lines[height-2], "30/30") {
		t.Errorf("end did not reach the last row:\n%s", strings.Join(lines, "\n"))
	}
	view.HandleInput("k")
	view.HandleInput("\x1b[H") // home
	if lines = plain(60); !strings.HasPrefix(lines[1], "│ row  w") {
		t.Errorf("home did not return to the top:\n%s", strings.Join(lines, "\n"))
	}
	if renders != 4 {
		t.Errorf("scrolling asked for %d renders, want 4", renders)
	}
	if builds != 1 {
		t.Errorf("scrolling laid the report out %d times, want once", builds)
	}

	// A wider screen widens the trace track, within bounds.
	if !strings.Contains(plain(100)[1], "w"+strconv.Itoa(100-4-sessionMaxLaneNameWidth-sessionLaneChrome)) {
		t.Errorf("wide screen did not widen the track: %q", plain(100)[1])
	}
	if !strings.Contains(plain(20)[1], "w"+strconv.Itoa(sessionMinTraceWidth)) {
		t.Errorf("narrow screen shrank the track below its minimum: %q", plain(20)[1])
	}
	if !strings.Contains(plain(400)[1], "w"+strconv.Itoa(sessionMaxTraceWidth)) {
		t.Errorf("huge screen grew the track past its maximum: %q", plain(400)[1])
	}

	builds = 0
	view.HandleInput("r")
	view.Render(60)
	view.Render(60)
	if snapshots != 1 || builds != 1 {
		t.Errorf("refresh took %d snapshots and %d layouts, want 1 and 1", snapshots, builds)
	}
	if got := view.Render(5); len(got) != 1 {
		t.Errorf("a sliver of width should render one clipped row, got %d", len(got))
	}
	for _, key := range []string{"\x1b", "q", "\x03"} {
		view.HandleInput(key)
	}
	if closed != 3 {
		t.Errorf("close keys closed the view %d times, want 3", closed)
	}
}
