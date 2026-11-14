package codingagent

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sessionLoadFixture writes a tool-heavy history outside the timed region.
func sessionLoadFixture(tb testing.TB) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "session.jsonl")
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, `{"type":"session","version":3,"id":"load-bench","cwd":"/tmp","timestamp":"2026-01-01T00:00:00.000Z"}`)
	text := strings.Repeat("session history ", 256)
	for i := 0; i < 10000; i++ {
		parent := "null"
		if i > 0 {
			parent = fmt.Sprintf("\"%08x\"", i-1)
		}
		fmt.Fprintf(w, "{\"type\":\"message\",\"id\":\"%08x\",\"parentId\":%s,\"timestamp\":\"2026-01-01T00:00:00.000Z\",\"message\":{\"role\":\"toolResult\",\"toolCallId\":\"call-%d\",\"toolName\":\"read\",\"content\":[{\"type\":\"text\",\"text\":\"%s\"}],\"isError\":false,\"timestamp\":1}}\n", i, parent, i, text)
	}
	if err := w.Flush(); err != nil {
		tb.Fatal(err)
	}
	if err := f.Close(); err != nil {
		tb.Fatal(err)
	}
	return path
}

// loadFixtureSession opens the fixture and releases its private snapshot at cleanup.
func loadFixtureSession(tb testing.TB, path string) *Session {
	tb.Helper()
	s, err := loadSessionFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if s.snapshot != nil {
			s.snapshot.close()
		}
	})
	return s
}

// widgetSession exposes the real session to widget restore without starting a model.
type widgetSession struct {
	InteractiveSessionHandle
	session *Session
}

// Inner returns the fixture's native session; no other handle method is used.
func (s widgetSession) Inner() *Session { return s.session }

// BenchmarkSessionLoad measures indexing and context restore, excluding fixture creation.
func BenchmarkSessionLoad(b *testing.B) {
	path := sessionLoadFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, err := loadSessionFile(path)
		if err != nil {
			b.Fatal(err)
		}
		if len(s.slots) != 10000 {
			b.Fatal(len(s.slots))
		}
		if s.snapshot != nil {
			s.snapshot.close()
		}
	}
}

// BenchmarkSessionRestoreTodo measures the widget hydration done before a resumed
// transcript paints, including reads of cold historical tool results.
func BenchmarkSessionRestoreTodo(b *testing.B) {
	s := loadFixtureSession(b, sessionLoadFixture(b))
	m := &InteractiveMode{opts: InteractiveModeOptions{SessionHandle: widgetSession{session: s}}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.restoreTodoWidget(nil)
	}
}

// TestSessionRestoreTodoSkipsUnrelatedHistory prevents widget restore from
// decoding every historical tool result just to discover there are no todo calls.
func TestSessionRestoreTodoSkipsUnrelatedHistory(t *testing.T) {
	s := loadFixtureSession(t, sessionLoadFixture(t))
	m := &InteractiveMode{opts: InteractiveModeOptions{SessionHandle: widgetSession{session: s}}}
	before := s.EntryReads()
	m.restoreTodoWidget(nil)
	if read := s.EntryReads() - before; read != 0 {
		t.Fatalf("todo restore decoded %d unrelated tool results", read)
	}
}
