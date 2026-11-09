package prompts

import (
	"strings"
	"testing"
)

// TestSystemEmbedsTheFile pins the contract main.go relies on: the binary carries
// the prompt from system.md, so an installed ompig-slim never reads the file at
// runtime.
func TestSystemEmbedsTheFile(t *testing.T) {
	got := System()
	if strings.TrimSpace(got) == "" {
		t.Fatal("the embedded prompt is empty; system.md did not reach the binary")
	}
	// The trimmed copy must match the raw embed, with no surrounding blanks.
	if got != strings.TrimSpace(systemMD) {
		t.Error("System() should return the embedded text with surrounding space trimmed")
	}
}

// TestSystemKeepsSectionStructure guards against the prompt silently collapsing
// into one paragraph: the tagged sections are what let a reader, and the model,
// tell policy from tool guidance.
func TestSystemKeepsSectionStructure(t *testing.T) {
	got := System()
	for _, section := range []string{"autonomy", "approach", "tools", "verification", "long_tasks", "communication"} {
		if !strings.Contains(got, "<"+section+">") {
			t.Errorf("prompt is missing the <%s> section", section)
		}
		if !strings.Contains(got, "</"+section+">") {
			t.Errorf("prompt is missing the </%s> close tag", section)
		}
	}
}

// TestSystemIsStable confirms repeated calls return identical text, so a prompt
// diff between two runs reflects a real change.
func TestSystemIsStable(t *testing.T) {
	if System() != System() {
		t.Error("System() is not stable between calls")
	}
}
