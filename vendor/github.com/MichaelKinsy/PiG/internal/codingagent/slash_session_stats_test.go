package codingagent

import (
	"slices"
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
	out := widthx.StripTerminalSequences(sessionInfoRenderer(sc)())

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
	if len(live) != 1 || len(live[0].Spans) != 2 {
		t.Fatalf("live spans = %+v", live)
	}
	// The first turn runs to the next request, the last ends where it began:
	// the 14 idle minutes before the user's next message are not work.
	if want := (SessionSpan{base + 1_000, base + 62_000}); live[0].Spans[0] != want {
		t.Errorf("first turn = %+v, want %+v", live[0].Spans[0], want)
	}
	if want := (SessionSpan{base + 62_000, base + 62_000}); live[0].Spans[1] != want {
		t.Errorf("last turn = %+v, want %+v", live[0].Spans[1], want)
	}
	loaded, err := manager.Load(session.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Accounting().UsageBreakdown; !slices.Equal(got[0].Spans, live[0].Spans) {
		t.Errorf("reloaded spans = %+v, live %+v", got[0].Spans, live[0].Spans)
	}
}

func TestSessionTraceTrack(t *testing.T) {
	plain := func(slot int, s string) string { return s }
	track := func(lane []SessionSpan, all ...SessionSpan) string {
		return sessionTraceTrack(sessionLane{spans: lane}, newTraceAxis(all), plain, func(s string) string { return s })
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
	axis := newTraceAxis([]SessionSpan{{1000, 1000 + 10*min}, {5000 + 40*min, 5000 + 50*min}})
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

func TestSessionDuration(t *testing.T) {
	for ms, want := range map[int64]string{
		45_000:      "45s",
		3_690_000:   "1h 1m",
		180_000_000: "2d 2h",
		5_000:       "5s",
		61_000:      "1m 1s",
	} {
		if got := sessionDuration(ms); got != want {
			t.Errorf("sessionDuration(%d) = %q, want %q", ms, got, want)
		}
	}
}
