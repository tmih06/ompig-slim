package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestWriteStreamsWhileArgumentsArrive is write.ts's streaming preview: while
// the content argument grows, the card draws numbered, highlighted rows and a
// `streaming` footer, so the write is visible before it is applied.
func TestWriteStreamsWhileArgumentsArrive(t *testing.T) {
	card := NewToolRendererCard(context.Background(), "write", "c", "", json.RawMessage(`{"path":"main.go"}`), CreateAllToolRenderers()["write"], nil)
	defer card.Dispose()
	c := card.Component
	c.MarkExecutionStarted()
	c.StartedAt = time.Now()

	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("line %02d", i))
		c.SetDefinitionArgs(json.RawMessage(fmt.Sprintf(`{"path":"main.go","content":%q}`, strings.Join(lines, "\n"))))
		out := widthx.StripAnsi(strings.Join(c.Render(80), "\n"))
		if !strings.Contains(out, "streaming") {
			t.Fatalf("step %d: no streaming footer: %s", i, out)
		}
		// The preview follows the newest rows, so the last line is always drawn
		// and the first has scrolled out of the window once it outgrows it.
		if !strings.Contains(out, fmt.Sprintf("line %02d", i)) {
			t.Fatalf("step %d: preview did not follow the stream: %s", i, out)
		}
		if i > 12 && strings.Contains(out, "line 01") {
			t.Fatalf("step %d: preview froze on the head: %s", i, out)
		}
	}
	c.SetArgsComplete()
	out := widthx.StripAnsi(strings.Join(c.Render(80), "\n"))
	if strings.Contains(out, "streaming") {
		t.Fatalf("a completed write still says streaming: %s", out)
	}
	// write.ts renderContentPreview re-anchors to the head of the file and caps
	// it at WRITE_PREVIEW_LINES, so the tail rows are gone.
	if !strings.Contains(out, "line 01") || strings.Contains(out, "line 20") {
		t.Fatalf("completed write did not show the capped file head: %s", out)
	}
}

// TestEditStreamsReplacementBeforeDiff is edit.ts renderStreamingFallback: until
// the computed diff lands, a replace-mode edit shows the replacement text.
func TestEditStreamsReplacementBeforeDiff(t *testing.T) {
	card := NewToolRendererCard(context.Background(), "edit", "c", "", json.RawMessage(`{"path":"main.go"}`), CreateAllToolRenderers()["edit"], nil)
	defer card.Dispose()
	c := card.Component
	c.MarkExecutionStarted()
	c.StartedAt = time.Now()

	c.SetDefinitionArgs(json.RawMessage(`{"path":"main.go","oldText":"a","newText":"added one\nadded two\n"}`))
	out := widthx.StripAnsi(strings.Join(c.Render(80), "\n"))
	for _, want := range []string{"Edit main.go", "+ added one", "+ added two"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q while streaming: %s", want, out)
		}
	}
}
