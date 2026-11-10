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
	"time"

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

// TestFileMentionCardSanitizesPath keeps a mention taken from the prompt from
// breaking the card: an escape sequence or an embedded newline must not paint
// colour or add a row, because the rail and the row count assume one row each.
func TestFileMentionCardSanitizesPath(t *testing.T) {
	card := &fileMentionCard{outputPad: 1, mentions: []FileMention{
		{Path: "READ\x1b[31mME\x1b[0m.md", LineCount: 2, Content: "x"},
		{Path: "evil\npath", LineCount: 1, Content: "x"},
	}}
	rows := card.Render(60)
	if len(rows) != 2 {
		t.Fatalf("a crafted path produced %d rows; want one row per mention: %v", len(rows), rows)
	}
	first := widthx.StripAnsi(rows[0])
	if !strings.Contains(first, "Read README.md (2 lines)") {
		t.Fatalf("escape sequence survived: %q", first)
	}
	if strings.Contains(widthx.StripAnsi(strings.Join(rows, "\n")), "\x1b[31m") {
		t.Fatal("raw SGR reached the card")
	}
	second := widthx.StripAnsi(rows[1])
	if !strings.Contains(second, "Read evil path (1 line)") {
		t.Fatalf("newline was not collapsed: %q", second)
	}
	for _, row := range rows {
		if widthx.VisibleWidth(row) > 60 {
			t.Fatalf("row exceeds width: %q", row)
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
	if typed := FileMentionsOf(original); len(typed) != 2 || typed[1].IsDirectory != true {
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
	if prepareFileMentions(context.Background(), "@README.md", "") != nil {
		t.Fatal("resolved a mention without a cwd")
	}
	cwd := mentionFixture(t)
	if mentions := prepareFileMentions(context.Background(), "see @README.md", cwd); mentions == nil || len(mentions.mentions) != 1 {
		t.Fatalf("mentions = %+v", mentions)
	}
	if prepareFileMentions(context.Background(), "nothing here", cwd) != nil {
		t.Fatal("unmentioned prompt produced a read")
	}
}

// TestAppendFileMentionCard checks the transcript draws the card for the
// auto-read message and falls back to nothing when the entry has no rows.
func TestAppendFileMentionCard(t *testing.T) {
	mode := &InteractiveMode{opts: InteractiveModeOptions{CWD: mentionFixture(t)}}
	mode.chatContainer = tui.NewContainer()
	mentions := readPromptFileMentions(context.Background(), "see @README.md and @src", mode.opts.CWD)
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
	mentions := prepareFileMentions(context.Background(), "please explain @README.md", mode.opts.CWD)
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
		if _, err := fmt.Fprint(w, "alpha\nbeta\ngamma\n"); err != nil {
			t.Errorf("write body: %v", err)
		}
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
		if _, err := fmt.Fprint(w, "body\n"); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	defer server.Close()
	mentions := prepareFileMentions(context.Background(), "see @"+server.URL+"/x and @README.md", "")
	if mentions == nil || len(mentions.mentions) != 1 || !mentions.mentions[0].IsURL {
		t.Fatalf("mentions = %+v", mentions)
	}
}

// TestDeliverPromptReadsOffLoop covers deliverPrompt's routing and slot
// contract: what the return value means, that the worker releases the slot
// exactly once, that a replaced session drops the delivery but still releases,
// and that a nil context reads inline. Every case binds the submission to a
// session that is not the mode's, so a delivery can never start a turn; the
// delivery itself (a real turn) is covered by
// TestFileMentionCardFollowsSubmittedPrompt.
func TestDeliverPromptReadsOffLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprint(w, "doc line\n"); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	defer server.Close()

	// staleSession is a current-looking session, so a submission bound to a
	// different value is dropped by the session check rather than by starting a
	// turn that the fixture cannot support.
	staleSession := &manualSession{}
	newSession := &manualSession{}

	// hold occupies the slot as finishPreflight does, so release is observable.
	hold := func(t *testing.T, mode *InteractiveMode, submission inputPreflight) {
		t.Helper()
		mode.preflightMu.Lock()
		mode.preflight = &submission
		mode.preflightMu.Unlock()
	}
	// awaitRelease drains the owner loop until the slot is free.
	awaitRelease := func(t *testing.T, mode *InteractiveMode) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for mode.preflightPending() {
			drainOwnerLoop(mode)
			if time.Now().After(deadline) {
				t.Fatal("the worker never released the preflight slot")
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	t.Run("local stays inline and reports completion", func(t *testing.T) {
		mode := resetWiringTest(t, nil)
		mode.opts.CWD = mentionFixture(t)
		mode.opts.SessionHandle = newSession
		submission := inputPreflight{ctx: context.Background(), session: staleSession}
		hold(t, mode, submission)
		if kept := mode.deliverPrompt(submission, "read @README.md", nil); kept {
			t.Fatal("a local mention was routed off the owner loop")
		}
		// The inline path delivered, so the caller releases the slot.
		mode.endPreflightDispatch()
		if mode.preflightPending() {
			t.Fatal("the preflight slot is still held")
		}
	})

	t.Run("url keeps the slot until the worker delivers", func(t *testing.T) {
		mode := resetWiringTest(t, nil)
		mode.opts.CWD = mentionFixture(t)
		mode.opts.SessionHandle = newSession
		submission := inputPreflight{ctx: context.Background(), session: staleSession}
		hold(t, mode, submission)
		if kept := mode.deliverPrompt(submission, "read @"+server.URL+"/doc", nil); !kept {
			t.Fatal("a URL prompt was delivered synchronously, freezing the owner loop")
		}
		// The worker owns the release now; it must free the slot exactly once.
		awaitRelease(t, mode)
		if mode.preflight != nil {
			t.Fatal("the slot was not cleared")
		}
	})

	t.Run("replaced session drops the delivery but still releases", func(t *testing.T) {
		mode := resetWiringTest(t, nil)
		mode.opts.CWD = mentionFixture(t)
		mode.opts.SessionHandle = newSession
		// A submission bound to a session that is no longer current.
		submission := inputPreflight{ctx: context.Background(), session: staleSession}
		hold(t, mode, submission)
		if kept := mode.deliverPrompt(submission, "read @"+server.URL+"/doc", nil); !kept {
			t.Fatal("a URL prompt was read inline")
		}
		awaitRelease(t, mode)
	})

	t.Run("nil context reads inline", func(t *testing.T) {
		mode := resetWiringTest(t, nil)
		mode.opts.CWD = mentionFixture(t)
		mode.opts.SessionHandle = newSession
		// A nil ctx cannot bound a worker fetch, so the URL is read on this
		// goroutine rather than being dropped; delivery then stops at the
		// session check, as every case here does.
		submission := inputPreflight{session: staleSession}
		hold(t, mode, submission)
		if kept := mode.deliverPrompt(submission, "read @"+server.URL+"/doc", nil); kept {
			t.Fatal("a nil context still went to the worker")
		}
	})
}

// TestQueuedPromptMessagesPairing pins the delivery unit: a prompt and its
// auto-read share one queue slot, so the one-at-a-time default cannot answer
// the prompt before the model has read the files it named.
func TestQueuedPromptMessagesPairing(t *testing.T) {
	cwd := mentionFixture(t)
	mentions := prepareFileMentions(context.Background(), "read @README.md", cwd)
	if mentions == nil {
		t.Fatal("no auto-read produced")
	}
	alone := queuedPromptMessages("plain prompt", nil, nil)
	if len(alone) != 1 || alone[0].User == nil {
		t.Fatalf("a prompt without mentions = %+v; want one user message", alone)
	}
	paired := queuedPromptMessages("read @README.md", nil, mentions)
	if len(paired) != 2 || paired[0].User == nil || paired[1].Custom == nil {
		t.Fatalf("a prompt with mentions = %+v; want the user message then its read", paired)
	}
	if paired[1].Custom["customType"] != FileMentionsCustomType {
		t.Fatalf("second message = %+v; want the mention card", paired[1].Custom)
	}
}

// TestQueueGroupDrainsTogether checks the queue slot the pairing relies on: a
// group is one drain unit under one-at-a-time mode, and separate submissions
// still drain one at a time.
func TestQueueGroupDrainsTogether(t *testing.T) {
	queue := agent.NewPendingMessageQueue(agent.QueueModeOneAtATime)
	user := agent.AgentMessage{User: &agent.UserMessage{Role: agent.RoleUser}}
	read := agent.AgentMessage{Custom: map[string]any{"customType": FileMentionsCustomType}}
	queue.EnqueueGroup(user, read)
	queue.EnqueueGroup(agent.AgentMessage{User: &agent.UserMessage{Role: agent.RoleUser}})

	if got := queue.Drain(); len(got) != 2 {
		t.Fatalf("the pair drained as %d messages; want both in one turn", len(got))
	}
	if got := queue.Drain(); len(got) != 1 {
		t.Fatalf("the next submission drained as %d messages; want its own turn", len(got))
	}
	if got := queue.Drain(); got != nil {
		t.Fatalf("drained %+v from an empty queue", got)
	}
}

// TestBeginSendMessagesWithKeepsPendingOnRefusal makes sure a refused run does
// not consume deferred next-turn messages: build runs only after the claim.
func TestBeginSendMessagesWithKeepsPendingOnRefusal(t *testing.T) {
	var built bool
	_, err := (&agent.Agent{}).BeginSendMessagesWith(context.Background(), func() []agent.AgentMessage {
		built = true
		return nil
	})
	if err == nil {
		t.Skip("an agent with no model accepted the run")
	}
	if built {
		t.Fatal("the batch was built before the run was claimed")
	}
}

// manualSession is an InteractiveSessionHandle stub: the embedded interface
// makes every method nil-safe, so a test can bind a submission to a distinct
// session without constructing a real one.
type manualSession struct{ InteractiveSessionHandle }

// tail returns the last n bytes of s, for failure messages.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
