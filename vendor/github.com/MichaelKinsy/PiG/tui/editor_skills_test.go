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
