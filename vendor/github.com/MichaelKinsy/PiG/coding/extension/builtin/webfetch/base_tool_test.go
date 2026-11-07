//go:build !pig_strip_webfetch

package webfetch

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
)

const articleHTML = `<html><head><title>Tool fixture</title></head><body><article>
<h1>Tool fetch proof</h1>
<p>TOOL_FETCH_PROOF: the extension rendered this article through the shared pipeline and returned it as tool text with details metadata.</p>
<p>A second paragraph keeps the rendering above the reader's quality gate so the native branch is the one that answers.</p>
</article></body></html>`

func serve(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL
}

// execute runs the tool with the given settings and parameters.
func execute(t *testing.T, ctx context.Context, settings extension.Settings, params string) (result extension.AgentToolResult, details *Details) {
	t.Helper()
	run := Execute
	if ctx == nil {
		ctx = context.Background()
	}
	options := Options{}
	if settings != nil {
		options.GetSettings = func() extension.Settings { return settings }
	}
	result, err := run(ctx, json.RawMessage(params), options)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if bound, ok := result.Details.(*Details); ok {
		details = bound
	}
	return result, details
}

// text returns the first text block of a result.
func text(result extension.AgentToolResult) string {
	for _, block := range result.Content {
		if value, ok := block.(ai.TextContent); ok {
			return value.Text
		}
	}
	return ""
}

func TestExecuteRendersHTMLWithDetails(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, articleHTML)
	})
	result, details := execute(t, nil, nil, `{"url":"`+base+`/article"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", text(result))
	}
	if details == nil || details.Method != "html-native" || details.Title != "Tool fixture" {
		t.Fatalf("details = %+v", details)
	}
	if details.URL != base+"/article" || details.Status != 200 || details.TotalLines == 0 {
		t.Fatalf("details = %+v", details)
	}
	body := text(result)
	for _, want := range []string{"Fetched " + base + "/article", "Title: Tool fixture", "TOOL_FETCH_PROOF"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestExecuteHonoursRawSelectorAndLineWindow(t *testing.T) {
	body := strings.Join([]string{"alpha", "bravo", "charlie", "delta"}, "\n")
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, body)
	})
	result, details := execute(t, nil, nil, `{"url":"`+base+`/x:2-3"}`)
	if details.Method != "text" || !strings.Contains(text(result), "bravo") || strings.Contains(text(result), "alpha") {
		t.Fatalf("line window: %+v\n%s", details, text(result))
	}
	if details.StartLine != 2 || details.EndLine != 3 {
		t.Fatalf("details = %+v", details)
	}

	raw, rawDetails := execute(t, nil, nil, `{"url":"`+base+`/x:raw"}`)
	if rawDetails.Method != "raw" || !strings.Contains(text(raw), "alpha\nbravo") {
		t.Fatalf("raw: %+v\n%s", rawDetails, text(raw))
	}
}

func TestExecuteTruncatesAndWritesArtifact(t *testing.T) {
	var builder strings.Builder
	for range 600 {
		builder.WriteString("a line of the long document with several words in it\n")
	}
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, builder.String())
	})
	result, details := execute(t, nil, nil, `{"url":"`+base+`/long.txt"}`)
	if !details.Truncated || details.FullOutputPath == "" {
		t.Fatalf("details = %+v", details)
	}
	if lines := strings.Count(text(result), "\n"); lines > maxOutputLines+10 {
		t.Fatalf("output holds %d lines", lines)
	}
	full, err := os.ReadFile(details.FullOutputPath)
	if err != nil {
		t.Fatalf("artifact: %v", err)
	}
	if !strings.Contains(string(full), "a line of the long document") || strings.Count(string(full), "\n") < 600 {
		t.Fatalf("artifact is not the whole rendering (%d bytes)", len(full))
	}
	os.Remove(details.FullOutputPath)
}

func TestExecuteSettingsAndConfigurationErrors(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "x")
	})
	// A disabled tool is a configuration problem: an error, not a result.
	if _, err := Execute(context.Background(), json.RawMessage(`{"url":"`+base+`/x"}`), Options{
		GetSettings: func() extension.Settings { return extension.Settings{"webFetch": map[string]any{"enabled": false}} },
	}); err == nil {
		t.Fatal("expected a configuration error for webFetch.enabled=false")
	}
	// An unsupported reader is a configuration problem too.
	if _, err := Execute(context.Background(), json.RawMessage(`{"url":"`+base+`/x"}`), Options{
		GetSettings: func() extension.Settings { return extension.Settings{"webFetch": map[string]any{"reader": "lynx"}} },
	}); err == nil {
		t.Fatal("expected a configuration error for an unsupported reader")
	}
}

func TestExecuteRuntimeFailuresAreIsError(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	result, details := execute(t, nil, nil, `{"url":"`+base+`/missing"}`)
	if !result.IsError || details == nil || details.URL != base+"/missing" {
		t.Fatalf("result = %+v details = %+v", result, details)
	}
	if !strings.HasPrefix(text(result), "Error: ") {
		t.Fatalf("text = %q", text(result))
	}

	// A bad URL is a runtime failure with the reason in the transcript.
	mismatch, _ := execute(t, nil, nil, `{"url":"ftp://example.com/x"}`)
	if !mismatch.IsError || !strings.Contains(text(mismatch), "scheme") {
		t.Fatalf("scheme failure = %q", text(mismatch))
	}
}

func TestExecutePropagatesCancellation(t *testing.T) {
	started := make(chan struct{})
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-started
		cancel()
	}()
	if _, err := Execute(ctx, json.RawMessage(`{"url":"`+base+`/stall"}`), Options{}); err == nil {
		t.Fatal("cancellation must reach the caller as an error")
	}
}

func TestExecuteImageResult(t *testing.T) {
	png := pngFixture(t)
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
	result, details := execute(t, nil, nil, `{"url":"`+base+`/a.png"}`)
	if details == nil || details.Method != "image" {
		t.Fatalf("details = %+v", details)
	}
	if !strings.Contains(text(result), "Image image/png") {
		t.Fatalf("text = %q", text(result))
	}
	for _, block := range result.Content {
		if _, ok := block.(ai.ImageContent); ok {
			return
		}
	}
	t.Fatal("no image block")
}

// pngFixture builds a valid 1x1 PNG so the image path reaches the resize step.
func pngFixture(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	canvas.Set(0, 0, color.RGBA{R: 0x20, G: 0x80, B: 0x40, A: 0xff})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// TestExecuteWindowNoteIsAccurate pins the "Showing lines" note to the window the
// model actually sees: the output truncation cuts it, so the note must not claim
// the whole document was shown or offer a range past the end.
func TestExecuteWindowNoteIsAccurate(t *testing.T) {
	const total = 600
	var builder strings.Builder
	for range total {
		builder.WriteString("a line of the long document with several words in it\n")
	}
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, builder.String())
	})

	result, details := execute(t, nil, nil, `{"url":"`+base+`/long.txt"}`)
	body := text(result)
	if !details.Truncated || details.TotalLines != total {
		t.Fatalf("details = %+v", details)
	}
	if !strings.Contains(body, "[Showing lines 1-300 of 600.") {
		t.Fatalf("window note is wrong:\n%s", tailOf(body))
	}
	if !strings.Contains(body, "Append :301-600 to the URL for the next range.") {
		t.Fatalf("next range is wrong:\n%s", tailOf(body))
	}
	os.Remove(details.FullOutputPath)

	// A range that reaches the end offers nothing further.
	last, _ := execute(t, nil, nil, `{"url":"`+base+`/long.txt:301-600"}`)
	lastBody := text(last)
	if !strings.Contains(lastBody, "[Showing lines 301-600 of 600.]") {
		t.Fatalf("tail window note:\n%s", tailOf(lastBody))
	}
	if strings.Contains(lastBody, "next range") {
		t.Fatalf("offered a range past the end:\n%s", tailOf(lastBody))
	}

	// A partial window names the real start.
	middle, _ := execute(t, nil, nil, `{"url":"`+base+`/long.txt:100-120"}`)
	if !strings.Contains(text(middle), "[Showing lines 100-120 of 600.") {
		t.Fatalf("middle window note:\n%s", tailOf(text(middle)))
	}
}

// TestExecuteDetailsReportBodyCap keeps the body-cap flag in the details even when
// the rendering itself fits the output limits.
func TestExecuteDetailsReportBodyCap(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, strings.Repeat("x", 4096))
	})
	result, details := execute(t, nil, extension.Settings{"webFetch": map[string]any{"maxBytes": float64(64)}}, `{"url":"`+base+`/big.txt"}`)
	if !details.Truncated {
		t.Fatalf("body cap not reported: %+v", details)
	}
	if !strings.Contains(text(result), "truncated at 64B") {
		t.Fatalf("text = %q", tailOf(text(result)))
	}
	// Output truncation did not happen: 64 bytes is well inside the 50 KiB head.
	if details.FullOutputPath != "" {
		t.Fatalf("unexpected artifact: %s", details.FullOutputPath)
	}
}

// tailOf returns the last few lines of a tool result for failure messages.
func tailOf(body string) string {
	lines := strings.Split(body, "\n")
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	return strings.Join(lines, "\n")
}
