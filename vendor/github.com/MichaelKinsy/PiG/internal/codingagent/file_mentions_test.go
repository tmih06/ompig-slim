package codingagent

// Tests for auto-read `@path` mentions: extraction, reading, truncation, the
// model-visible text, and the transcript card OMP draws as
// `├─ Read path (N lines)`.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// mentionFixture builds a working directory with a text file, a directory and a
// binary file, and returns the cwd.
func mentionFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "src")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"b.go", "a.go"} {
		if err := os.WriteFile(filepath.Join(sub, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(sub, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), []byte{0x00, 0x01, 0x02}, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestExtractFileMentions covers the boundary, quoting and punctuation rules
// that keep prose such as an email address from becoming a file read.
func TestExtractFileMentions(t *testing.T) {
	cases := map[string][]string{
		"look at @README.md please":                 {"README.md"},
		"@README.md":                                {"README.md"},
		"(@src/a.go), then @src/b.go.":              {"src/a.go", "src/b.go"},
		`@"my notes.txt" and @'other notes.txt'`:    {"my notes.txt", "other notes.txt"},
		"mail me at user@example.com":               nil,
		"@README.md and again @README.md":           {"README.md"},
		"nothing mentioned here":                    nil,
		"@`src/a.go`":                               {"src/a.go"},
		"@README.md\n@src/a.go":                     {"README.md", "src/a.go"},
		"compare @README.md with @src/a.go quickly": {"README.md", "src/a.go"},
	}
	for prompt, want := range cases {
		got := ExtractFileMentions(prompt)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%q: mentions = %v, want %v", prompt, got, want)
		}
	}
}

// TestReadFileMentions covers file content, directory listing, binary skipping
// and the ordering the card draws.
func TestReadFileMentions(t *testing.T) {
	cwd := mentionFixture(t)
	mentions := ReadFileMentions(context.Background(), []string{"README.md", "src", "blob.bin", "missing.md"}, cwd)
	if len(mentions) != 3 {
		t.Fatalf("mentions = %+v; want the missing path skipped", mentions)
	}
	readme := mentions[0]
	if readme.Path != "README.md" || readme.LineCount != 4 || readme.Content != "one\ntwo\nthree\n" {
		t.Fatalf("readme = %+v", readme)
	}
	src := mentions[1]
	if !src.IsDirectory || src.LineCount != 3 || src.Content != "nested/\na.go\nb.go" {
		t.Fatalf("directory = %+v", src)
	}
	binary := mentions[2]
	if binary.Skipped != "binary" || !strings.Contains(binary.Content, "skipped auto-read") || binary.ByteSize != 3 {
		t.Fatalf("binary = %+v", binary)
	}
	if got := fileMentionDetail(readme); got != "4 lines" {
		t.Fatalf("readme detail = %q", got)
	}
	if got := fileMentionDetail(src); got != "3 entries" {
		t.Fatalf("directory detail = %q", got)
	}
	if got := fileMentionDetail(binary); got != "skipped: binary, 3B" {
		t.Fatalf("binary detail = %q", got)
	}
}

// TestReadFileMentionsTruncates checks a long file is cut with the read tool's
// own notice, so the auto-read and a manual read report the same text.
func TestReadFileMentionsTruncates(t *testing.T) {
	cwd := t.TempDir()
	var content strings.Builder
	for i := range 2500 {
		fmt.Fprintf(&content, "line %d\n", i+1)
	}
	if err := os.WriteFile(filepath.Join(cwd, "big.txt"), []byte(content.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	mentions := ReadFileMentions(context.Background(), []string{"big.txt"}, cwd)
	if len(mentions) != 1 {
		t.Fatalf("mentions = %+v", mentions)
	}
	if mentions[0].LineCount != 2501 || !strings.Contains(mentions[0].Content, "[Truncated:") {
		t.Fatalf("long file not truncated: lines=%d tail=%q", mentions[0].LineCount, tail(mentions[0].Content, 120))
	}
}

// TestFileMentionsText is the model-visible form: one <file path> block per
// mention in prompt order.
func TestFileMentionsText(t *testing.T) {
	text := FileMentionsText([]FileMention{{Path: "a.go", Content: "A"}, {Path: "src", Content: "b.go", IsDirectory: true}})
	want := "<file path=\"a.go\">\nA\n</file>\n<file path=\"src\">\nb.go\n</file>"
	if text != want {
		t.Fatalf("text = %q; want %q", text, want)
	}
	if FileMentionsText(nil) != "" {
		t.Fatal("no mentions rendered text")
	}
}

// TestFileMentionCardRendersTree checks the tree the user asked for, drawn
// entirely inside the terminal width.
func TestFileMentionCardRendersTree(t *testing.T) {
	card := &fileMentionCard{mentions: []FileMention{
		{Path: "README.md", LineCount: 123, Content: "x"},
		{Path: "src", LineCount: 12, IsDirectory: true, Content: "x"},
	}}
	rows := card.Render(80)
	if len(rows) != 2 {
		t.Fatalf("rows = %v", rows)
	}
	first, second := strings.TrimSpace(widthx.StripAnsi(rows[0])), strings.TrimSpace(widthx.StripAnsi(rows[1]))
	if !strings.HasPrefix(first, "├─") || !strings.Contains(first, "Read README.md (123 lines)") {
		t.Fatalf("first row = %q", rows[0])
	}
	if !strings.HasPrefix(second, "└─") || !strings.Contains(second, "Read src (12 entries)") {
		t.Fatalf("second row = %q", rows[1])
	}
	single := (&fileMentionCard{mentions: []FileMention{{Path: "a.go", LineCount: 1, Content: "x"}}}).Render(80)
	plain := ""
	if len(single) == 1 {
		plain = widthx.StripAnsi(single[0])
	}
	if !strings.Contains(plain, "└─") || !strings.Contains(plain, "1 line") {
		t.Fatalf("single row = %v", single)
	}
	// A row never wraps: the tree rail only stays aligned at one row per entry.
	for _, row := range card.Render(12) {
		if widthx.VisibleWidth(row) > 12 {
			t.Fatalf("row exceeds width: %q", row)
		}
	}
	if (&fileMentionCard{}).Render(80) != nil {
		t.Fatal("empty card rendered rows")
	}
}

// TestFileMentionCardIndent checks the tree sits one indent in from the prompt
// block and stays within the terminal at that indent.
func TestFileMentionCardIndent(t *testing.T) {
	mentions := []FileMention{{Path: "README.md", LineCount: 123, Content: "x"}}
	flat := (&fileMentionCard{mentions: mentions}).Render(80)
	indented := (&fileMentionCard{mentions: mentions, outputPad: 1}).Render(80)
	if len(flat) != 1 || len(indented) != 1 {
		t.Fatalf("flat=%v indented=%v", flat, indented)
	}
	if !strings.HasPrefix(widthx.StripAnsi(flat[0]), "└─") {
		t.Fatalf("zero padding did not start the row: %q", flat[0])
	}
	if !strings.HasPrefix(widthx.StripAnsi(indented[0]), " └─") {
		t.Fatalf("outputPad 1 did not indent the row: %q", indented[0])
	}
	// The indent counts against the width, so a padded row is never too wide.
	for _, row := range (&fileMentionCard{mentions: mentions, outputPad: 1}).Render(14) {
		if widthx.VisibleWidth(row) > 14 {
			t.Fatalf("indented row exceeds width: %q", row)
		}
	}
	// A padding wider than the terminal collapses instead of emitting a blank row.
	for _, row := range (&fileMentionCard{mentions: mentions, outputPad: 200}).Render(10) {
		if widthx.VisibleWidth(row) > 10 {
			t.Fatalf("over-wide padding produced a long row: %q", row)
		}
	}
}

// TestFileMentionsRoundTrip checks a resumed session rebuilds the card from the
// entry's details, whether they arrive typed or as decoded JSON.
func TestFileMentionsRoundTrip(t *testing.T) {
	original := []FileMention{{Path: "README.md", LineCount: 3, Content: "a\nb\nc"}, {Path: "src", LineCount: 2, IsDirectory: true, Content: "x"}}
	encoded, err := json.Marshal(map[string]any{"mentions": original})
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	got := FileMentionsOf(decoded)
	if len(got) != 2 || got[0] != original[0] || got[1] != original[1] {
		t.Fatalf("round trip = %+v", got)
	}
	if typed := FileMentionsOf([]FileMention(original)); len(typed) != 2 || typed[1].IsDirectory != true {
		t.Fatalf("typed details = %+v", typed)
	}
	if FileMentionsOf(nil) != nil {
		t.Fatal("nil details produced mentions")
	}
	message := FileMentionsMessage(original)
	if message.Custom["customType"] != FileMentionsCustomType || message.Custom["display"] != true {
		t.Fatalf("message = %+v", message.Custom)
	}
	if content, _ := message.Custom["content"].(string); !strings.Contains(content, `<file path="README.md">`) {
		t.Fatalf("message content = %q", content)
	}
}

// TestPrepareFileMentionsNeedsCwd keeps a session without a working directory
// from resolving a mention against the process directory.
func TestPrepareFileMentionsNeedsCwd(t *testing.T) {
	m := &InteractiveMode{}
	if m.prepareFileMentions(context.Background(), "@README.md") != nil {
		t.Fatal("resolved a mention without a cwd")
	}
	m.opts.CWD = mentionFixture(t)
	if mentions := m.prepareFileMentions(context.Background(), "see @README.md"); mentions == nil || len(mentions.mentions) != 1 {
		t.Fatalf("mentions = %+v", mentions)
	}
	if m.prepareFileMentions(context.Background(), "nothing here") != nil {
		t.Fatal("unmentioned prompt produced a read")
	}
}

// TestAppendFileMentionCard checks the transcript draws the card for the
// auto-read message and falls back to nothing when the entry has no rows.
func TestAppendFileMentionCard(t *testing.T) {
	mode := &InteractiveMode{opts: InteractiveModeOptions{CWD: mentionFixture(t)}}
	mode.chatContainer = tui.NewContainer()
	mentions := mode.readPromptFileMentions(context.Background(), "see @README.md and @src")
	if len(mentions) != 2 {
		t.Fatalf("mentions = %+v", mentions)
	}
	entry := CustomMessageEntry{CustomType: FileMentionsCustomType, Display: true, Details: map[string]any{"mentions": mentions}}
	mode.appendCustomMessage(entry)
	transcript := widthx.StripAnsi(strings.Join(mode.chatContainer.Render(80), "\n"))
	for _, want := range []string{"Read README.md (4 lines)", "Read src (3 entries)", "├─", "└─"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("transcript missing %q: %q", want, transcript)
		}
	}
	empty := &InteractiveMode{}
	empty.chatContainer = tui.NewContainer()
	empty.appendCustomMessage(CustomMessageEntry{CustomType: FileMentionsCustomType, Display: true})
	if len(empty.chatContainer.Children()) != 0 {
		t.Fatal("an entry without rows drew a card")
	}
}

// TestFileMentionCardFollowsSubmittedPrompt drives the live event order: the
// user message arrives first and the auto-read right after it, so the rows sit
// directly under the prompt in the transcript.
func TestFileMentionCardFollowsSubmittedPrompt(t *testing.T) {
	mode := resetWiringTest(t, nil)
	mode.opts.CWD = mentionFixture(t)
	// Production reads this from settings (GetOutputPad defaults to one cell), so
	// the card is indented with the prompt block it belongs to.
	mode.outputPad = 1
	mentions := mode.prepareFileMentions(context.Background(), "please explain @README.md")
	if mentions == nil {
		t.Fatal("no auto-read produced")
	}
	mode.handleAgentEvent(agent.MessageStartEvent{Message: agent.AgentMessage{User: &agent.UserMessage{
		Role:    agent.RoleUser,
		Content: ai.UserContentBlocks{ai.TextContent{Text: "please explain @README.md"}},
	}}})
	mode.handleAgentEvent(agent.MessageStartEvent{Message: mentions.message})
	rows := mode.chatContainer.Render(80)
	// SanitizeDisplayLine collapses whitespace, so the indent is read from the
	// raw row and the text from the sanitized one.
	raw := widthx.StripAnsi(rows[len(rows)-1])
	last := tui.SanitizeDisplayLine(rows[len(rows)-1])
	if !strings.HasPrefix(raw, " └─ ") {
		t.Fatalf("card is not indented under the prompt: %q", raw)
	}
	if !strings.Contains(last, "└─ Read README.md (4 lines)") {
		t.Fatalf("card is not the row under the prompt: %q", last)
	}
	if !strings.Contains(widthx.StripAnsi(strings.Join(rows, "\n")), "please explain @README.md") {
		t.Fatal("the prompt is not in the transcript")
	}
}

// TestReadURLMentions fetch a real document over loopback, so the URL path is
// exercised end to end without touching a network the tests do not own.
func TestReadURLMentions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "alpha\nbeta\ngamma\n")
	}))
	defer server.Close()

	url := server.URL + "/doc.txt"
	if !isFileMentionURL(url) {
		t.Fatalf("%q is not detected as a URL", url)
	}
	if isFileMentionURL("src/a.go") {
		t.Fatal("a relative path was detected as a URL")
	}
	mentions := ReadFileMentions(context.Background(), []string{url}, t.TempDir())
	if len(mentions) != 1 {
		t.Fatalf("mentions = %+v", mentions)
	}
	got := mentions[0]
	if !got.IsURL || got.Path != url {
		t.Fatalf("mention = %+v", got)
	}
	// The row reports the document's own line count, and the content is read's
	// rendering: the Fetched header plus the body.
	if got.LineCount != 3 {
		t.Fatalf("line count = %d; want 3", got.LineCount)
	}
	for _, want := range []string{"Fetched " + url, "alpha", "gamma"} {
		if !strings.Contains(got.Content, want) {
			t.Fatalf("content missing %q: %q", want, got.Content)
		}
	}
	if detail := fileMentionDetail(got); detail != "3 lines" {
		t.Fatalf("detail = %q", detail)
	}
}

// TestReadURLMentionFailure keeps a broken URL as a named row with the reason,
// unlike a path that turned out to be prose.
func TestReadURLMentionFailure(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	mentions := ReadFileMentions(context.Background(), []string{server.URL + "/missing"}, t.TempDir())
	if len(mentions) != 1 {
		t.Fatalf("mentions = %+v", mentions)
	}
	if !mentions[0].IsURL || mentions[0].Skipped != "fetchFailed" || !strings.Contains(mentions[0].Content, "skipped auto-read") {
		t.Fatalf("failure row = %+v", mentions[0])
	}
	if detail := fileMentionDetail(mentions[0]); detail != "fetch failed" {
		t.Fatalf("detail = %q", detail)
	}
}

// TestMentionsNeedNetwork routes a URL mention off the owner loop and keeps a
// local-only mention inline.
func TestMentionsNeedNetwork(t *testing.T) {
	for _, tc := range []struct {
		prompt string
		want   bool
	}{
		{"read @README.md", false},
		{"read @https://example.com/doc", true},
		{"read @http://example.com and @README.md", true},
		{"nothing mentioned", false},
	} {
		if got := mentionsNeedNetwork(tc.prompt); got != tc.want {
			t.Errorf("%q: needsNetwork = %t, want %t", tc.prompt, got, tc.want)
		}
	}
}

// TestPrepareFileMentionsKeepsURLWithoutCwd makes sure a URL still reads when a
// session has no working directory, while a relative path does not resolve
// against the process directory.
func TestPrepareFileMentionsKeepsURLWithoutCwd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "body\n")
	}))
	defer server.Close()
	m := &InteractiveMode{}
	mentions := m.prepareFileMentions(context.Background(), "see @"+server.URL+"/x and @README.md")
	if mentions == nil || len(mentions.mentions) != 1 || !mentions.mentions[0].IsURL {
		t.Fatalf("mentions = %+v", mentions)
	}
}

// TestDeliverPromptReadsOffLoop is the routing rule deliverPrompt applies: a
// URL mention set is read on a worker and reports that it kept the preflight
// slot, while a local-only set is read inline and releases it at once. The
// delivery half is covered by TestFileMentionCardFollowsSubmittedPrompt; these
// cases stop at the routing decision so no turn is started.
func TestDeliverPromptReadsOffLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "doc line\n")
	}))
	defer server.Close()
	cwd := mentionFixture(t)
	for _, tc := range []struct {
		name    string
		prompt  string
		offLoop bool
	}{
		{"local", "read @README.md", false},
		{"url", "read @" + server.URL + "/doc", true},
		{"mixed", "read @README.md and @" + server.URL + "/doc", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mentionsNeedNetwork(tc.prompt); got != tc.offLoop {
				t.Fatalf("needsNetwork = %t, want %t", got, tc.offLoop)
			}
			// The same read either way, so routing cannot change the rows.
			mentions := ReadFileMentions(context.Background(), ExtractFileMentions(tc.prompt), cwd)
			if len(mentions) == 0 {
				t.Fatal("no mentions read")
			}
			for _, mention := range mentions {
				if mention.Content == "" {
					t.Fatalf("empty content for %q", mention.Path)
				}
			}
		})
	}
}

// tail returns the last n bytes of s, for failure messages.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
