package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// skillTestProvider includes a built-in to catch accidental mid-prompt command completion.
func skillTestProvider() *CombinedProvider {
	return NewCombinedProvider([]SlashCommand{{Name: "model"}, {Name: "skill:review"}, {Name: "skill:test"}, {Name: "skill:team/check"}}, "", "")
}

// TestMidPromptSkills checks replacement boundaries, filtering and native query planning.
func TestMidPromptSkills(t *testing.T) {
	p := skillTestProvider()
	for _, before := range []string{"please /", "/skill:review /", "please\u3000/", "please /skill:team/"} {
		lines := []string{before + " suffix"}
		result, work := prepareNativeAutocomplete(p, lines, 0, len(before), false)
		if work != nil || result == nil || len(result.Items) == 0 {
			t.Fatalf("%q: missing immediate skills: %+v", before, result)
		}
		for _, item := range result.Items {
			if !strings.HasPrefix(item.Value, "skill:") {
				t.Fatalf("mid-prompt built-in: %+v", item)
			}
		}
		item := result.Items[0]
		edited, _, col := p.ApplyCompletion(lines, 0, len(before), item, result.Prefix)
		want := strings.TrimSuffix(before, result.Prefix) + "/" + item.Value + "  suffix"
		if edited[0] != want || col != len(want)-len(" suffix") {
			t.Fatalf("got %q @%d; want %q", edited[0], col, want)
		}
	}
	multiline := p.GetSuggestions(context.Background(), []string{"use /skill:review", "/"}, 1, 1, AutocompleteSuggestionOptions{})
	if multiline == nil {
		t.Fatal("missing multiline skill completion")
	}
	for _, item := range multiline.Items {
		if !strings.HasPrefix(item.Value, "skill:") {
			t.Fatal("built-in suggested after first line")
		}
	}
	for _, before := range []string{"/model /", "!echo /", "$\techo /", "https://example/", "please /tmp/"} {
		if result := p.GetSuggestions(context.Background(), []string{before}, 0, len(before), AutocompleteSuggestionOptions{}); result != nil {
			for _, item := range result.Items {
				if strings.HasPrefix(item.Value, "skill:") {
					t.Fatalf("%q suggested skills", before)
				}
			}
		}
	}
}

// TestSkillSelectionContinuesPrompt verifies two sequential selections don't submit,
// and that badges are display-only, atomic on deletion, and undoable.
func TestSkillSelectionContinuesPrompt(t *testing.T) {
	e := NewEditor()
	e.SetAutocomplete(skillTestProvider())
	e.SetText("/rev")
	e.RefreshAutocomplete()
	if !e.AutocompleteOpen() || e.AutocompleteAccept() {
		t.Fatal("skill selection must stay in editor")
	}
	e.HandleInput("/tes")
	if !e.AutocompleteOpen() || e.AutocompleteAccept() {
		t.Fatal("second skill must also stay in editor")
	}
	want := "/skill:review /skill:test "
	if e.Text() != want {
		t.Fatalf("text = %q", e.Text())
	}
	rendered := strings.Join(e.Render(80), "\n")
	if strings.Contains(rendered, "/skill:") || !strings.Contains(rendered, "◆ review") || !strings.Contains(rendered, "◆ test") {
		t.Fatalf("missing badges: %q", rendered)
	}
	for _, row := range e.Render(18) {
		if widthx.VisibleWidth(row) > 18 {
			t.Fatalf("row exceeds width: %q", row)
		}
	}
	e.backspace() // separator
	e.backspace() // atomic skill
	if e.Text() != "/skill:review " {
		t.Fatalf("partial skill left behind: %q", e.Text())
	}
	e.HandleInput("\x1f") // Ctrl+_: undo
	if e.Text() != strings.TrimSuffix(want, " ") {
		t.Fatalf("undo lost skill: %q", e.Text())
	}
}

// TestCompletedSkillContinuesAsProse checks natural queries after Tab, not
// merely whether accepting the menu closed it at that instant.
func TestCompletedSkillContinuesAsProse(t *testing.T) {
	e := NewEditor()
	e.SetAutocomplete(skillTestProvider())
	e.asyncAutocomplete = &AsyncAutocompleteProvider{}
	for _, text := range []string{"/skill:review ", "/skill:review fix this", "please /skill:review fix"} {
		e.SetText(text)
		if e.naturalAutocompleteContext() {
			t.Errorf("%q triggered native suggestions in prose", text)
		}
		if natural, _ := e.naturalAsyncAutocomplete(); natural {
			t.Errorf("%q triggered async suggestions in prose", text)
		}
	}
	for _, text := range []string{"/skill:review /tes", "/skill:review @file", "/model "} {
		e.SetText(text)
		if !e.naturalAutocompleteContext() {
			t.Errorf("%q lost native completion", text)
		}
		if natural, _ := e.naturalAsyncAutocomplete(); !natural {
			t.Errorf("%q lost async completion", text)
		}
	}
}

// TestForcedSkillSelectionDoesNotReopenCompletion keeps a single Tab completion
// from falling through to file completion after the skill token is complete.
func TestForcedSkillSelectionDoesNotReopenCompletion(t *testing.T) {
	e := NewEditor()
	e.SetAutocomplete(skillTestProvider())
	e.SetText("/rev")
	e.applySingleForcedCompletion(AutocompleteItem{Value: "skill:review"}, "/rev")
	if e.AutocompleteOpen() {
		t.Fatal("skill completion reopened autocomplete")
	}
	if got := e.Text(); got != "/skill:review " {
		t.Fatalf("text = %q", got)
	}
}

// TestAsyncSkillSelection stays in the composer when an extension wraps the provider.
func TestAsyncSkillSelection(t *testing.T) {
	e := NewEditor()
	base := skillTestProvider()
	e.SetAutocomplete(base)
	e.SetAsyncApply(func(func()) {})
	e.SetAsyncAutocomplete(&AsyncAutocompleteProvider{
		ApplyCompletion: func(_ context.Context, lines []string, row, col int, item AutocompleteItem, prefix string) ([]string, int, int, error) {
			result, row, col := base.ApplyCompletion(lines, row, col, item, prefix)
			return result, row, col, nil
		},
	}, context.Background(), func(func()) {}, func(work AutocompleteWork) {
		apply, err := work(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		apply()
	}, func(err error) { t.Fatal(err) })
	e.SetText("please /rev")
	e.setAutocompleteItems([]AutocompleteItem{{Value: "skill:review"}})
	e.autocompletePrefix = "/rev"
	e.autocompleteQueryCursor = e.cursor
	called := false
	e.AcceptAutocomplete(func(submit bool) {
		called = true
		if submit {
			t.Fatal("async skill selection submitted")
		}
	})
	if !called || e.Text() != "please /skill:review " {
		t.Fatalf("async completion = %q; called=%v", e.Text(), called)
	}
	e.SetText("please /skill:review /")
	if natural, _ := e.naturalAsyncAutocomplete(); !natural {
		t.Fatal("second skill didn't trigger async completion")
	}
}

// TestAsyncSkillTabDoesNotQueryFiles drives Tab and subsequent prose through
// the provider worker boundary. A post-accept query would reopen the popup.
func TestAsyncSkillTabDoesNotQueryFiles(t *testing.T) {
	e := NewEditor()
	base := skillTestProvider()
	e.SetAutocomplete(base)
	e.SetAsyncApply(func(apply func()) { apply() })
	queries := 0
	e.SetAsyncAutocomplete(&AsyncAutocompleteProvider{
		GetSuggestions: func(context.Context, []string, int, int, bool) (*AutocompleteSuggestions, error) {
			queries++
			return &AutocompleteSuggestions{Items: []AutocompleteItem{{Value: "directory/"}}}, nil
		},
		ApplyCompletion: func(_ context.Context, lines []string, row, col int, item AutocompleteItem, prefix string) ([]string, int, int, error) {
			lines, row, col = base.ApplyCompletion(lines, row, col, item, prefix)
			return lines, row, col, nil
		},
	}, context.Background(), func(work func()) { work() }, func(work AutocompleteWork) {
		apply, err := work(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		apply()
	}, func(err error) { t.Fatal(err) })
	e.SetText("/rev")
	e.setAutocompleteItems([]AutocompleteItem{{Value: "skill:review"}})
	e.autocompletePrefix = "/rev"
	e.autocompleteQueryCursor = e.cursor
	e.HandleInput("\t")
	e.HandleInput("fix this")
	if queries != 0 || e.AutocompleteOpen() || e.Text() != "/skill:review fix this" {
		t.Fatalf("skill Tab/prose triggered files: queries=%d popup=%v text=%q", queries, e.AutocompleteOpen(), e.Text())
	}
}

// TestSkillBadgeBackground checks the tinted pill has no invisible-prefix padding.
func TestSkillBadgeBackground(t *testing.T) {
	e := NewEditor()
	e.SetAutocomplete(skillTestProvider())
	e.SetText("/skill:review ")
	for _, col := range []int{-1, 13, 14} {
		rendered := e.skillDisplay(e.Text(), col, false)
		if !strings.Contains(rendered, " ◆ review \x1b[22m\x1b[39m\x1b[49m") {
			t.Fatalf("background extends beyond the label at cursor %d: %q", col, rendered)
		}
	}
	if widthx.VisibleWidth(e.skillDisplay(e.Text(), -1, false)) != widthx.VisibleWidth(" ◆ review  ") {
		t.Fatal("badge still reserves space for the slash prefix")
	}
}

// TestCompactSkillCoordinates checks adjacent prose, wrapping and cursor offsets use
// badge widths while the editable buffer continues to hold the original invocations.
func TestCompactSkillCoordinates(t *testing.T) {
	e := NewEditor()
	e.SetAutocomplete(skillTestProvider())
	e.SetText("/skill:review /skill:test next")
	want := " ◆ review   ◆ test  next"
	if got := widthx.VisibleWidth(e.skillDisplay(e.Text(), -1, false)); got != widthx.VisibleWidth(want) {
		t.Fatalf("width=%d; want %d (no reserved prefix spaces)", got, widthx.VisibleWidth(want))
	}
	for _, source := range []int{0, 13, 14, 25, jsstring.Length(e.Text())} {
		display := e.skillColumn(e.Text(), source)
		if got := e.skillSourceColumn(e.Text(), display); got != source {
			t.Fatalf("source column %d round-tripped to %d", source, got)
		}
	}
	// The full source is too wide, but the compact badges and prose fit this row.
	if chunks := e.wrapLine(e.Text(), widthx.VisibleWidth(want)); len(chunks) != 1 {
		t.Fatalf("wrapped compact line prematurely: %+v", chunks)
	}
	e.SetText("/skill:review next")
	rendered := e.skillDisplay(e.Text(), -1, false)
	reset := "\x1b[22m\x1b[39m\x1b[49m"
	if !strings.Contains(rendered, "◆ review "+reset+" next") {
		t.Fatalf("extra space before prose: %q", rendered)
	}
	if chunks := e.wrapLine(e.Text(), 10); len(chunks) < 2 || chunks[0].text != "/skill:review" {
		t.Fatalf("badge not wrapped atomically: %+v", chunks)
	}
	e.Render(80)
	e.HandleMouse(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft, X: 11 + e.paddingX, Y: 1, Width: 80})
	if e.cursor != [2]int{0, 14} {
		t.Fatalf("mouse used hidden prefix width: %v", e.cursor)
	}
	e.SetText("/skill:review x\nabcdefghijklmno")
	e.cursor = [2]int{0, jsstring.Length(e.lines[0])}
	visual := e.buildVisualLineMap(80)
	e.moveToVisualLine(visual, 0, 1)
	if e.cursor != [2]int{1, 12} {
		t.Fatalf("vertical movement used hidden prefix width: %v", e.cursor)
	}
}

// TestSkillArgumentCompletionInCommandArgs checks the fallback the skill gate
// needs: in a command whose arguments forbid skill tokens, a slash-prefixed
// argument still reaches that command's own synchronous argument completer, and
// no skill or command name is offered instead.
func TestSkillArgumentCompletionInCommandArgs(t *testing.T) {
	p := NewSlashOnlyProvider([]SlashCommand{{
		Name: "plan",
		GetArgumentCompletions: func(prefix string) []AutocompleteItem {
			if prefix != "/pr" {
				return nil
			}
			return []AutocompleteItem{{Value: "/proceed", Label: "/proceed"}}
		},
	}, {Name: "skill:review"}})
	lines := []string{"/plan /pr"}
	if AllowsSkillTokens(lines[0]) {
		t.Fatal("a command line should not allow skill tokens")
	}
	result := p.GetSuggestions(context.Background(), lines, 0, len(lines[0]), AutocompleteSuggestionOptions{})
	if result == nil || len(result.Items) == 0 || result.Items[0].Value != "/proceed" {
		t.Fatalf("slash-prefixed argument lost its completion: %+v", result)
	}
	if result.Prefix != "/pr" {
		t.Fatalf("prefix = %q; want the argument text", result.Prefix)
	}
}

// TestSkillGateCachesLeadingRun checks the AllowsSkillTokens cache answers the
// same as the uncached function for every gate shape, and that text after the
// leading run cannot change the answer.
func TestSkillGateCachesLeadingRun(t *testing.T) {
	e := NewEditor()
	e.SetAutocomplete(skillTestProvider())
	for _, text := range []string{
		"",
		"plain prompt",
		"!echo /skill:review",
		"/model /skill:review",
		"/skill:review",
		"/skill:review and more /skill:test",
		"$$ /skill:review",
		"${VAR} /skill:review",
		"$ /skill:review",
	} {
		e.SetText(text)
		if got, want := e.skillTokenGate(), AllowsSkillTokens(text); got != want {
			t.Errorf("%q: gate = %t, want %t", text, got, want)
		}
		// Changing text after the leading run must not change the gate.
		first := e.lines[0]
		e.lines[0] = first + " trailing /skill:test"
		if got, want := e.skillTokenGate(), AllowsSkillTokens(text); got != want {
			t.Errorf("%q: gate changed with trailing text: %t, want %t", text, got, want)
		}
	}
}

// TestWrapCacheSkillLines checks the wrap cache stays live when the draft holds
// a skill: unchanged lines keep their cached chunks, and the gate or the
// registry changing re-wraps them.
func TestWrapCacheSkillLines(t *testing.T) {
	e := NewEditor()
	e.SetAutocomplete(skillTestProvider())
	e.SetText("plain one\n/skill:review\nplain two")
	// The badge (10 cells) fits this width; the 13-cell source token does not.
	const width = 12
	first := e.wrappedChunks(width)
	if len(first) != 3 || len(first[1]) != 1 {
		t.Fatalf("a badge that fits was wrapped: %+v", first)
	}
	second := e.wrappedChunks(width)
	if &second[1][0] != &e.wrapCacheChunks[1][0] {
		t.Fatal("an unchanged draft was re-wrapped")
	}
	// A registry that no longer knows the skill must re-wrap the line: the badge
	// collapses back to the literal source, which no longer fits.
	e.SetAutocomplete(NewCombinedProvider([]SlashCommand{{Name: "model"}}, "", ""))
	third := e.wrappedChunks(width)
	if len(third[1]) != 2 || !strings.Contains(third[1][0].text, "/skill:") {
		t.Fatalf("stale badge served after the registry changed: %+v", third[1])
	}

	// A gate flip has the same effect without any registry change: the whole
	// draft is command input, so the token is literal text and wraps at its own
	// characters, while an allowed prompt keeps it atomic as one badge.
	e.SetAutocomplete(skillTestProvider())
	e.SetText("/model /skill:review")
	closed := e.wrappedChunks(width)
	e.lines[0] = "please /skill:review"
	open := e.wrappedChunks(width)
	if lastOpen := open[0][len(open[0])-1]; lastOpen.text != "/skill:review" {
		t.Fatalf("an allowed token was not kept atomic: %+v", open[0])
	}
	for _, chunk := range closed[0] {
		if chunk.text == "/skill:review" {
			t.Fatalf("a closed gate kept the token atomic: %+v", closed[0])
		}
	}
	if !strings.Contains(e.skillDisplay(e.lines[0], -1, false), "◆ review") {
		t.Fatal("an allowed prompt drew no badge")
	}
}

// TestWrapCacheNotReusedAcrossWidths reproduces the stale-width reuse: a draft
// holding a skill never published its cache, so the cached width advanced while
// the stored chunks kept the old one, and a later call at that width wrapped
// nothing and painted rows far past the terminal.
func TestWrapCacheNotReusedAcrossWidths(t *testing.T) {
	e := NewEditor()
	e.SetAutocomplete(skillTestProvider())
	prose := strings.Repeat("word ", 30)
	e.SetText("plain line\n" + prose)
	e.wrappedChunks(80)
	e.SetText("/skill:review\n" + prose)
	e.wrappedChunks(80)
	e.wrappedChunks(40) // the call that used to advance the cached width only
	for i, chunks := range e.wrappedChunks(40) {
		for _, chunk := range chunks {
			if got := widthx.VisibleWidth(chunk.text); got > 40 {
				t.Fatalf("line %d chunk %q is %d cells wide, wider than 40", i, chunk.text, got)
			}
		}
	}
}

// TestTabFallsThroughToPathCompletion checks a trailing slash token in prose
// that names no skill still lets Tab reach path completion.
func TestTabFallsThroughToPathCompletion(t *testing.T) {
	e := NewEditor()
	// No skills registered: the trailing token can never match one.
	e.SetAutocomplete(NewCombinedProvider([]SlashCommand{{Name: "model"}}, "", ""))
	e.SetText("look in /prose")
	e.cursor = [2]int{0, len("/prose") + len("look in ")}
	if !e.skillSlashContext("look in /prose") {
		t.Fatal("the trailing token is not a skill context")
	}
	e.HandleInput("\t")
	if e.AutocompleteOpen() {
		// A skill popup with no candidates must not be left open.
		for _, item := range e.autocompleteItems {
			if item.Value != "" && len(item.Value) > 0 && item.Value[0] != '@' {
				t.Fatalf("Tab opened a command popup for prose: %+v", e.autocompleteItems)
			}
		}
	}
}

// BenchmarkSkillSpanScanLargeBuffer bounds the repeated-call workload skillSpans
// adds to a render: a large pasted buffer with /skill: on many lines used to
// rejoin the whole draft per call through e.Text(). The gate now caches on the
// leading run, so the cost tracks the token count, not the buffer length.
func BenchmarkSkillSpanScanLargeBuffer(b *testing.B) {
	e := NewEditor()
	e.SetAutocomplete(skillTestProvider())
	lines := make([]string, 0, 500)
	lines = append(lines, "plain opening line")
	for i := range 500 {
		if i%10 == 0 {
			lines = append(lines, "use /skill:review here and /skill:test too")
			continue
		}
		lines = append(lines, "an ordinary prose line with no tokens at all")
	}
	e.SetText(strings.Join(lines, "\n"))
	b.ResetTimer()
	for range b.N {
		if spans := e.skillSpans(e.lines[1]); len(spans) == 0 {
			b.Fatal("no spans found")
		}
	}
}

// TestSkillDraftExclusions keeps unknown skills and literal command arguments unstyled.
func TestSkillDraftExclusions(t *testing.T) {
	e := NewEditor()
	e.SetAutocomplete(skillTestProvider())
	for _, draft := range []string{"/skill:unknown ", "!echo /skill:review ", "/model /skill:review "} {
		e.SetText(draft)
		if strings.Contains(strings.Join(e.Render(80), "\n"), "◆") {
			t.Fatalf("styled literal draft %q", draft)
		}
	}
}
