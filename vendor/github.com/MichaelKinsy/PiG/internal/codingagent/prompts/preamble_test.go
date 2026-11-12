package prompts

import (
	"strings"
	"testing"
)

// TestPreambleOverride guards the seam an embedding program uses to supply its
// own base prompt: the text must reach the rendered prompt, and the tagged
// sections must still compose around it.
func TestPreambleOverride(t *testing.T) {
	original := Preamble
	t.Cleanup(func() { Preamble = original })

	Preamble = "<autonomy>\nPersist until done.\n</autonomy>"
	out := BuildDefaultPrompt(Options{Cwd: "/tmp", Tools: []string{"read"}})
	if !strings.Contains(out, "Persist until done.") {
		t.Fatalf("override missing from prompt:\n%s", out)
	}
	if !strings.Contains(out, "<rules>") {
		t.Error("sections should still compose around a replaced preamble")
	}
	if strings.Contains(out, "coding assistant running inside ompig-slim") {
		t.Error("the stock preamble should be replaced, not kept")
	}
}

// TestNoHarnessDocumentationContext prevents explicit legacy documentation paths
// from adding a self-help index to either prompt-building entry point.
func TestNoHarnessDocumentationContext(t *testing.T) {
	options := Options{Cwd: "/tmp"}
	for _, section := range BuildSystemPromptSections(options) {
		if section.Name == "docs" {
			t.Fatal("documentation section is still present")
		}
	}
	out := BuildDefaultPrompt(options)
	for _, unwanted := range []string{"<docs>", "harness-docs", "harness-readme", "harness-examples"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("documentation leaked into prompt: %s", unwanted)
		}
	}
}

// TestPreambleEmptyFallsBack pins the guard that keeps a cleared variable from
// producing a prompt with no opening paragraph.
func TestPreambleEmptyFallsBack(t *testing.T) {
	original := Preamble
	t.Cleanup(func() { Preamble = original })

	for _, empty := range []string{"", "   \n\t "} {
		Preamble = empty
		out := BuildDefaultPrompt(Options{Cwd: "/tmp", Tools: []string{"read"}})
		if !strings.Contains(out, defaultPreamble) {
			t.Errorf("preamble %q should fall back to the default, got:\n%s", empty, out)
		}
	}
}

// TestPreambleNotSharedWithOptions confirms the override is read at render time
// rather than captured, so setting it before a session builds its prompt is
// enough.
func TestPreambleNotSharedWithOptions(t *testing.T) {
	original := Preamble
	t.Cleanup(func() { Preamble = original })

	options := Options{Cwd: "/tmp", Tools: []string{"read"}}
	Preamble = "first preamble"
	if out := BuildDefaultPrompt(options); !strings.Contains(out, "first preamble") {
		t.Fatal("the first override did not apply")
	}
	Preamble = "second preamble"
	if out := BuildDefaultPrompt(options); !strings.Contains(out, "second preamble") {
		t.Error("a new override should apply to the same Options")
	}
}
