package codingagent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MichaelKinsy/PiG/tui"
)

// compactedToolSession leaves a large tool-output prefix outside a small or
// empty kept range, as a real old session after compaction does.
func compactedToolSession(tb testing.TB, firstKept string) *Session {
	tb.Helper()
	path := sessionLoadFixture(tb)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		tb.Fatal(err)
	}
	_, err = fmt.Fprintf(file, `{"type":"compaction","id":"compact","parentId":"0000270f","timestamp":"2026-01-01T00:00:00.000Z","firstKeptEntryId":%q,"summary":"Saved summary","tokensBefore":100000}`+"\n", firstKept)
	if err != nil {
		tb.Fatal(err)
	}
	if err := file.Close(); err != nil {
		tb.Fatal(err)
	}
	return loadFixtureSession(tb, path)
}

// resumedCacheMode uses the real transcript rebuild with default cache notices
// enabled; no model or extension is needed to paint retained session entries.
func resumedCacheMode(session *Session) *InteractiveMode {
	return &InteractiveMode{opts: InteractiveModeOptions{SessionHandle: widgetSession{session: session}}, chatContainer: tui.NewContainer(), tuiInst: tui.NewWithOutput(io.Discard, 100, 24), toolByID: map[string]*tui.ToolExecutionComponent{}}
}

// TestCompactedResumeLeavesToolHistoryCold catches full-history reads caused by
// cache-notice hydration even though those tool bodies cannot affect notices.
func TestCompactedResumeLeavesToolHistoryCold(t *testing.T) {
	for _, firstKept := range []string{"0000270c", "compact", "absent", ""} {
		t.Run(firstKept, func(t *testing.T) {
			session := compactedToolSession(t, firstKept)
			mode := resumedCacheMode(session)
			if !mode.showCacheMissNotices() {
				t.Fatal("fixture must exercise enabled cache notices")
			}
			before := session.EntryReads()
			mode.renderSessionEntriesWithHistory(true)
			if reads := session.EntryReads() - before; reads != 0 {
				t.Fatalf("resume decoded %d compacted tool results for cache notices", reads)
			}
			if mode.chatContainer.IsEmpty() {
				t.Fatal("resume lost its retained summary")
			}
		})
	}
}

// cacheNoticeLabels compares notices by stable entry ID rather than decoded
// pointer identity, so independent whole-history and index reads can be checked.
func cacheNoticeLabels(entries []SessionEntry, prices ModelPriceSource) map[string]string {
	stats := make([]cacheStatsEntry, 0, len(entries))
	for _, entry := range entries {
		item := cacheStatsEntry{kind: entry.Base().Type}
		if message, ok := asMessage(entry); ok {
			item.message = message.Message.Assistant
		}
		if item.kind == "usage" {
			if usage, ok := decodeUsageEntry(entry.Raw()); ok {
				item.usage = &usage
			}
		}
		stats = append(stats, item)
	}
	misses := collectCacheMisses(stats, prices)
	labels := make(map[string]string)
	for i, item := range stats {
		if item.message != nil {
			labels[entries[i].Base().ID] = formatCacheMissNotice(misses[item.message])
		}
	}
	return labels
}

// TestCacheNoticeIndexPreservesFileOrder compares against the original full
// history scan: orphan responses still count, warm requests participate, and
// compaction/branch-summary boundaries reset the cache comparison.
func TestCacheNoticeIndexPreservesFileOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.jsonl")
	data := `{"type":"session","version":3,"id":"cache","cwd":"/tmp"}
{"type":"message","id":"cached","message":{"role":"assistant","content":[],"provider":"fixture","model":"one","usage":{"cacheRead":80000},"timestamp":1}}
{"type":"message","id":"tool","parentId":"cached","message":{"role":"toolResult","toolName":"read","toolCallId":"c","content":[{"type":"text","text":"unrelated"}],"timestamp":1}}
{"type":"message","id":"user","parentId":"tool","message":{"role":"user","content":"kept prompt","timestamp":1}}
{"type":"message","id":"miss","parentId":"user","message":{"role":"assistant","content":[],"provider":"fixture","model":"one","usage":{"input":80000,"cost":{"input":0.4}},"timestamp":2}}
{"type":"compaction","id":"compact","parentId":"miss","firstKeptEntryId":"user","summary":"summary"}
{"type":"message","id":"reset","parentId":"compact","message":{"role":"assistant","content":[],"provider":"fixture","model":"one","usage":{"input":80000},"timestamp":3}}
{"type":"message","id":"orphan","parentId":"cached","message":{"role":"assistant","content":[],"provider":"fixture","model":"one","usage":{"cacheRead":90000},"timestamp":4}}
{"type":"message","id":"fork-miss","parentId":"reset","message":{"role":"assistant","content":[],"provider":"fixture","model":"one","usage":{"input":90000,"cost":{"input":0.4}},"timestamp":5}}
{"type":"branch_summary","id":"summary","parentId":"fork-miss","summary":"branch summary"}
{"type":"message","id":"summary-reset","parentId":"summary","message":{"role":"assistant","content":[],"provider":"fixture","model":"one","usage":{"input":90000},"timestamp":6}}
{"type":"usage","id":"warm","parentId":"summary-reset","kind":"cache_warm","provider":"fixture","model":"one","timestamp":"2026-01-01T00:00:00.000Z","usage":{"cacheRead":90000}}
{"type":"message","id":"warm-miss","parentId":"warm","message":{"role":"assistant","content":[],"provider":"fixture","model":"one","usage":{"input":90000,"cost":{"input":0.4}},"timestamp":7}}
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	session := loadFixtureSession(t, path)
	for _, prices := range []ModelPriceSource{nil, func(string, string) float64 { return 2 }} {
		want := cacheNoticeLabels(session.GetEntries(), prices)
		got := cacheNoticeLabels(session.cacheStatsEntries(), prices)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("filtered notices differ from whole-history scan: got %v, want %v", got, want)
		}
		for _, id := range []string{"miss", "fork-miss", "warm-miss"} {
			if got[id] == "" {
				t.Fatal("fixture did not produce expected cache miss", id)
			}
		}
		if got["reset"] != "" || got["summary-reset"] != "" {
			t.Fatal("cache comparison crossed a summary boundary", got)
		}
	}
}

// BenchmarkCompactedResume measures the transcript hot path independently of
// loading/indexing; history outside the kept range must stay cold on rebuild.
func BenchmarkCompactedResume(b *testing.B) {
	session := compactedToolSession(b, "0000270c")
	mode := resumedCacheMode(session)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mode.chatContainer.Clear()
		mode.renderSessionEntriesWithHistory(true)
	}
}
