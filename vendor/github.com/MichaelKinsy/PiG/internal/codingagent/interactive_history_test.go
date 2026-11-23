package codingagent

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func pagedFixture(tb testing.TB, users int) (string, []string) {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "paged.jsonl")
	var b strings.Builder
	b.WriteString(`{"type":"session","version":3,"id":"paged","cwd":"/tmp"}` + "\n")
	ids := []string{}
	parent := "null"
	write := func(id, line string) {
		b.WriteString(line + "\n")
		ids = append(ids, id)
		parent = fmt.Sprintf("%q", id)
	}
	for i := 0; i < users; i++ {
		id := fmt.Sprintf("u%03d", i)
		write(id, fmt.Sprintf(`{"type":"message","id":%q,"parentId":%s,"timestamp":"2026-01-01T00:00:00.000Z","message":{"role":"user","content":%q,"timestamp":%d}}`, id, parent, fmt.Sprintf("older prompt %d", i), i))
		if i == 2 {
			write("oc0", fmt.Sprintf(`{"type":"compaction","id":"oc0","parentId":%s,"timestamp":"2026-01-01T00:00:00.000Z","firstKeptEntryId":"","summary":"older summary","tokensBefore":4000}`, parent))
		}
	}
	write("pc", fmt.Sprintf(`{"type":"compaction","id":"pc","parentId":%s,"timestamp":"2026-01-01T00:00:00.000Z","firstKeptEntryId":"","summary":"paged summary","tokensBefore":9000}`, parent))
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("k%d", i)
		write(id, fmt.Sprintf(`{"type":"message","id":%q,"parentId":%s,"timestamp":"2026-01-01T00:00:00.000Z","message":{"role":"user","content":%q,"timestamp":%d}}`, id, parent, fmt.Sprintf("kept prompt %d", i), 100+i))
	}
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		tb.Fatal(err)
	}
	return path, ids
}

func TestBranchEntriesBeforePagesInOrderOnce(t *testing.T) {
	path, ids := pagedFixture(t, 25)
	session := loadFixtureSession(t, path)
	want := ids
	shown := map[string]struct{}{"u012": {}}
	var got []string
	before := "k1"
	for pages := 0; pages < 10; pages++ {
		page, more, found := session.BranchEntriesBefore(before, shown, 7)
		if !found {
			t.Fatalf("boundary %q reported not on path", before)
		}
		if len(page) == 0 {
			if more {
				t.Fatal("empty page with more entries claimed")
			}
			break
		}
		for _, e := range page {
			id := e.Base().ID
			if _, dup := shown[id]; dup {
				t.Fatalf("paging delivered %q twice", id)
			}
			shown[id] = struct{}{}
			got = append(got, id)
		}
		before = page[0].Base().ID
		if !more {
			break
		}
	}
	var desc []string
	for i := len(want) - 2; i >= 0; i-- {
		if want[i] != "u012" {
			desc = append(desc, want[i])
		}
	}
	var expected []string
	for len(desc) > 0 {
		n := min(7, len(desc))
		for i := n - 1; i >= 0; i-- {
			expected = append(expected, desc[i])
		}
		desc = desc[n:]
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("paged order = %v, want %v", got, expected)
	}
	if !slices.Contains(got, "u000") {
		t.Fatalf("paging never delivered the root entry: %v", got)
	}
}

func TestBranchEntriesBeforeSurvivesAppend(t *testing.T) {
	path, _ := pagedFixture(t, 25)
	session := loadFixtureSession(t, path)
	shown := map[string]struct{}{}
	first, more, found := session.BranchEntriesBefore("k1", shown, 7)
	if !found || !more || len(first) != 7 || first[0].Base().ID != "u020" {
		t.Fatalf("first page = %d entries from %q (more=%v found=%v), want 7 from u020", len(first), first[0].Base().ID, more, found)
	}
	if err := session.AppendEntry(map[string]any{
		"type": "message", "id": "appended", "parentId": "k1",
		"message": map[string]any{"role": "user", "content": "new prompt", "timestamp": 200},
	}); err != nil {
		t.Fatal(err)
	}
	for _, e := range first {
		shown[e.Base().ID] = struct{}{}
	}
	second, more, found := session.BranchEntriesBefore(first[0].Base().ID, shown, 7)
	if !found || len(second) != 7 || second[0].Base().ID != "u013" {
		t.Fatalf("page after append = %d entries from %q, want 7 from u013", len(second), second[0].Base().ID)
	}
	if more != true {
		t.Fatal("older entries remain, more must be true")
	}
}

func TestBranchEntriesBeforeBoundary(t *testing.T) {
	path, _ := pagedFixture(t, 3)
	session := loadFixtureSession(t, path)
	if page, _, found := session.BranchEntriesBefore("missing", nil, 7); found || page != nil {
		t.Fatal("off-path boundary must not produce a page")
	}
	page, more, found := session.BranchEntriesBefore("u000", map[string]struct{}{}, 7)
	if !found || len(page) != 0 || more {
		t.Fatalf("root boundary: page=%d more=%v found=%v, want empty/false/true", len(page), more, found)
	}
}

func TestTranscriptHistoryPagingDecodesBounded(t *testing.T) {
	session := compactedToolSession(t, "")
	mode := resumedCacheMode(session)
	before := session.EntryReads()
	mode.renderSessionEntriesWithHistory(true)
	if reads := session.EntryReads() - before; reads != 0 {
		t.Fatalf("resume decoded %d hidden entries", reads)
	}
	if mode.transcriptHistory == nil {
		t.Fatal("resume installed no transcript history window")
	}
	before = session.EntryReads()
	mode.pageTranscriptHistory()
	if reads := session.EntryReads() - before; reads <= 0 || reads > 3*transcriptPageSize {
		t.Fatalf("scroll step decoded %d entries, want 1..%d in bounded pages", reads, 3*transcriptPageSize)
	}
	for _, ch := range mode.chatContainer.Children() {
		if _, isText := ch.(*tui.UserMessageComponent); isText {
			t.Fatal("undisplayed tool results painted a chat child")
		}
	}
	if mode.transcriptHistory == nil || len(mode.transcriptHistory.units) < 2 {
		t.Fatalf("history units = %+v, want page units before the painted window", mode.transcriptHistory)
	}
}

func transcriptText(m *InteractiveMode, width int) string {
	return widthx.StripAnsi(strings.Join(m.chatContainer.Render(width), "\n"))
}

func TestTranscriptHistoryPagingRestoresOlderMessages(t *testing.T) {
	path, _ := pagedFixture(t, 30)
	session := loadFixtureSession(t, path)
	mode := resumedCacheMode(session)
	mode.renderSessionEntriesWithHistory(true)
	initial := transcriptText(mode, 100)
	if !strings.Contains(initial, "kept prompt 0") || !strings.Contains(initial, "Compacted from") {
		t.Fatalf("resume did not paint kept tail and summary: %s", initial)
	}
	if strings.Contains(initial, "older prompt") {
		t.Fatal("resume painted hidden history")
	}
	calls := session.projectionCalls.Load()
	mode.pageTranscriptHistory()
	text := transcriptText(mode, 100)
	for _, want := range []string{"older prompt 0", "older prompt 29"} {
		if !strings.Contains(text, want) {
			t.Fatalf("paged transcript missing %q: %s", want, text)
		}
	}
	if strings.Count(text, "Compacted from") != 2 {
		t.Fatalf("compaction cards = %d, want live card plus older marker: %s", strings.Count(text, "Compacted from"), text)
	}
	if i, j := strings.Index(text, "older prompt 0"), strings.Index(text, "older prompt 29"); i >= j {
		t.Fatal("paged history is out of order")
	}
	if a, b, c := strings.Index(text, "older prompt 2"), strings.Index(text, "Compacted from"), strings.Index(text, "older prompt 3"); !(a < b && b < c) {
		t.Fatalf("older compaction marker not chronological: %d %d %d", a, b, c)
	}
	if strings.Index(text, "older prompt 29") > strings.Index(text, "kept prompt 0") {
		t.Fatal("paged history did not splice above the kept tail")
	}
	if session.projectionCalls.Load() != calls {
		t.Fatal("paging touched the model projection")
	}
	if mode.transcriptHistory.units[0].more {
		t.Fatal("paging left the frontier open after the last page")
	}
}

func TestTranscriptHistoryOrphanedToolResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orphan.jsonl")
	data := `{"type":"session","version":3,"id":"orphan","cwd":"/tmp"}
{"type":"message","id":"call","message":{"role":"assistant","content":[{"type":"toolCall","id":"tc1","name":"read","arguments":{"path":"/f"}}],"stopReason":"toolUse","timestamp":1}}
{"type":"compaction","id":"c1","parentId":"call","firstKeptEntryId":"tr","summary":"cut call","tokensBefore":10}
{"type":"message","id":"tr","parentId":"c1","message":{"role":"toolResult","toolCallId":"tc1","toolName":"read","content":[{"type":"text","text":"result body"}],"timestamp":2}}
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	session := loadFixtureSession(t, path)
	mode := resumedCacheMode(session)
	mode.renderSessionEntriesWithHistory(true)
	if len(mode.transcriptHistory.pendingToolResults) != 1 {
		t.Fatalf("kept result was not stashed: %+v", mode.transcriptHistory.pendingToolResults)
	}
	mode.pageTranscriptHistory()
	if len(mode.transcriptHistory.pendingToolResults) != 0 {
		t.Fatal("page did not consume the stashed result")
	}
	if len(mode.toolOrder) != 1 {
		t.Fatalf("page restored %d tool cards, want 1", len(mode.toolOrder))
	}
	mode.toolOrder[0].SetExpanded(true)
	if got := transcriptText(mode, 100); !strings.Contains(got, "result body") {
		t.Fatalf("paged tool card lacks its result: %s", got)
	}
}

func TestTranscriptHistoryStaleBoundary(t *testing.T) {
	path, _ := pagedFixture(t, 5)
	session := loadFixtureSession(t, path)
	mode := resumedCacheMode(session)
	mode.renderSessionEntriesWithHistory(true)
	mode.transcriptHistory.units[0].startID = "detached"
	mode.pageTranscriptHistory()
	if mode.transcriptHistory != nil {
		t.Fatal("stale boundary must drop the history window")
	}
}

func TestTranscriptHistoryResetOnRepaint(t *testing.T) {
	path, _ := pagedFixture(t, 5)
	session := loadFixtureSession(t, path)
	mode := resumedCacheMode(session)
	mode.renderSessionEntriesWithHistory(true)
	mode.pageTranscriptHistory()
	if len(mode.transcriptHistory.units) != 2 {
		t.Fatal("page did not register a unit")
	}
	mode.chatContainer.Clear()
	mode.renderSessionEntriesWithHistory(true)
	if mode.transcriptHistory == nil || len(mode.transcriptHistory.units) != 1 {
		t.Fatal("repaint did not reset the history window")
	}
	if !mode.transcriptHistory.units[0].more {
		t.Fatal("repainted window must reopen the paging frontier")
	}
}

func TestTranscriptHistoryPagingSkipsUndisplayedEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	var b strings.Builder
	b.WriteString(`{"type":"session","version":3,"id":"usage","cwd":"/tmp"}` + "\n")
	parent := "null"
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("u%d", i)
		fmt.Fprintf(&b, `{"type":"usage","id":%q,"parentId":%s,"timestamp":"2026-01-01T00:00:00.000Z","kind":"tokens","usage":{"input":%d}}`+"\n", id, parent, i)
		parent = fmt.Sprintf("%q", id)
	}
	fmt.Fprintf(&b, `{"type":"compaction","id":"c","parentId":%s,"firstKeptEntryId":"","summary":"s","tokensBefore":1}`+"\n", parent)
	fmt.Fprintf(&b, `{"type":"message","id":"k","parentId":"c","message":{"role":"user","content":"kept","timestamp":9}}`+"\n")
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		t.Fatal(err)
	}
	session := loadFixtureSession(t, path)
	mode := resumedCacheMode(session)
	mode.renderSessionEntriesWithHistory(true)
	mode.pageTranscriptHistory()
	if mode.transcriptHistory == nil || len(mode.transcriptHistory.units) != 2 {
		t.Fatalf("usage page did not consume its interval: %+v", mode.transcriptHistory)
	}
	if mode.transcriptHistory.units[0].more {
		t.Fatal("usage page left the frontier open at the path root")
	}
	if got := transcriptText(mode, 100); !strings.Contains(got, "kept") {
		t.Fatalf("painted window lost its kept prompt: %q", got)
	}
	mode.pageTranscriptHistory()
	if len(mode.transcriptHistory.units) != 2 {
		t.Fatal("exhausted paging invented another unit")
	}
}

func TestOfferTranscriptHistoryPageIsRegularOnly(t *testing.T) {
	path, _ := pagedFixture(t, 5)
	session := loadFixtureSession(t, path)
	mode := resumedCacheMode(session)
	mode.renderSessionEntriesWithHistory(true)
	if mode.transcriptHistory == nil {
		t.Fatal("no history window installed")
	}
	if !mode.offerTranscriptHistoryPage("\x1b[7^") {
		t.Fatal("ctrl+home did not page in regular mode")
	}
	mode.editor = tui.NewEditor()
	mode.editor.SetText("draft")
	if mode.offerTranscriptHistoryPage("\x1b[7^") {
		t.Fatal("page key stole the editor's ctrl+home")
	}
}
