package tui

import (
	"strings"
	"testing"
)

// TestGoalSkillCompletion verifies goal objectives use the same skill completion
// and display as prose, without replacing the goal command or its surrounding text.
func TestGoalSkillCompletion(t *testing.T) {
	for _, prefix := range []string{"/goal /skill:rev", "/goal set /skill:rev", "/goal create fix this /skill:rev", "/goal\n/skill:rev"} {
		t.Run(prefix, func(t *testing.T) {
			e := NewEditor()
			e.SetAutocomplete(skillTestProvider())
			e.SetText(prefix)
			e.RefreshAutocomplete()
			if !e.AutocompleteOpen() {
				t.Fatalf("%q: missing skill suggestions", prefix)
			}
			if e.AutocompleteAccept() {
				t.Fatal("skill acceptance must not submit the goal")
			}
			want := strings.TrimSuffix(prefix, "/skill:rev") + "/skill:review "
			if e.Text() != want {
				t.Fatalf("text = %q; want %q", e.Text(), want)
			}
			rendered := strings.Join(e.Render(80), "\n")
			if !strings.Contains(rendered, "◆ review") {
				t.Fatalf("missing skill badge: %q", rendered)
			}
		})
	}
}

// TestGoalSkillContextBoundary keeps similarly named commands and shell input
// literal; only the whitespace-delimited goal command carries a prompt.
func TestGoalSkillContextBoundary(t *testing.T) {
	for _, text := range []string{"/goalpost /skill:review", "/goals /skill:review", "/goal/skill:review", "! /goal /skill:review", "/model /skill:review"} {
		if AllowsSkillTokens(text) {
			t.Errorf("%q unexpectedly allows skill tokens", text)
		}
	}
	for _, text := range []string{"/goal\t/skill:review", "  /goal /skill:review"} {
		if !AllowsSkillTokens(text) {
			t.Errorf("%q must allow skill tokens", text)
		}
	}
}
