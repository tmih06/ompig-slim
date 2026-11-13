package codingagent

import (
	"context"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui"
)

// TestBuiltinArgumentSuggestions exercises the actual host wiring, including
// empty prefixes, descriptions, filtering, and the boundary to later arguments.
func TestBuiltinArgumentSuggestions(t *testing.T) {
	m := &InteractiveMode{}
	m.opts.CWD = t.TempDir()
	m.opts.AgentDir = t.TempDir()
	provider := m.buildAutocompleteProvider()
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"/subagents ", "on,off,status,hub,menu,settings"},
		{"/agents ", "on,off,status,hub,menu,settings"},
		{"/subagents o", "on,off"},
		{"/subagents ST", "status"},
		{"/subagents unknown", ""},
		{"/subagents on ", ""},
		{"/usage ", "show,reset,auto-reset"},
		{"/usage reset ", ""},
		{"/reload ", "--explain"},
		{"/name ", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			result := provider.GetSuggestions(context.Background(), []string{tc.input}, 0, len(tc.input), tui.AutocompleteSuggestionOptions{})
			var values []string
			if result != nil {
				for _, item := range result.Items {
					values = append(values, item.Value)
					if item.Label == "" || item.Description == "" {
						t.Errorf("option lacks label or purpose: %+v", item)
					}
				}
			}
			if got := strings.Join(values, ","); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestLeadingAbsolutePathIsOrdinaryPrompt ensures filesystem paths are sent as
// ordinary prompts instead of being rejected as unknown commands.
func TestLeadingAbsolutePathIsOrdinaryPrompt(t *testing.T) {
	for _, text := range []string{"/tmp/project/file", "/tmp/project/file explain this", "/new/file.go", "/file.txt explain this", "/tmp", "/", "./file.go explain this", "~/file.go", "C:\\project\\file.go", "explain /not-a-command"} {
		m := &InteractiveMode{editor: tui.NewEditor(), slashRegistry: NewSlashRegistry()}
		m.setupEditorSubmitHandler(context.Background())
		var submitted string
		m.onInputCallback = func(text string) { submitted = text }
		m.editor.SetText(text)
		m.editor.HandleInput("\r")
		if submitted != text {
			t.Fatalf("path or ordinary text %q was not submitted: %q", text, submitted)
		}
	}
}

// TestUnknownSlashSubmissionStaysLocal exercises Enter and direct submission
// during compaction, before either can hand an unknown command to the model.
func TestUnknownSlashSubmissionStaysLocal(t *testing.T) {
	for _, text := range []string{"/does-not-exist", "/does-not-exist args", "/skill:missing", "/skill:team/missing"} {
		for _, direct := range []bool{false, true} {
			m := &InteractiveMode{editor: tui.NewEditor(), slashRegistry: NewSlashRegistry(), isCompacting: direct}
			m.setupEditorSubmitHandler(context.Background())
			submitted := false
			m.onInputCallback = func(string) { submitted = true }
			m.editor.SetText(text)
			if direct {
				m.handleSubmit(context.Background(), text)
			} else {
				m.editor.HandleInput("\r")
			}
			if submitted || len(m.pendingUserInputs) != 0 || len(m.compactionQueue) != 0 {
				t.Errorf("direct=%t: unknown command %q escaped submission validation", direct, text)
			}
			if m.editor.Text() != text {
				t.Errorf("direct=%t: rejected draft lost: %q", direct, m.editor.Text())
			}
		}
	}
}

// TestUnknownSlashFollowUpStaysLocal covers the active follow-up key's separate
// queue path; rejection must leave the draft and compaction queue untouched.
func TestUnknownSlashFollowUpStaysLocal(t *testing.T) {
	m := resetWiringTest(t, nil)
	m.slashRegistry = NewSlashRegistry()
	m.isCompacting = true
	m.editor.SetText("/does-not-exist")
	if err := m.handleEditorAction(context.Background(), actionFollowUp, "\x11"); err != nil {
		t.Fatal(err)
	}
	if len(m.compactionQueue) != 0 || m.editor.Text() != "/does-not-exist" {
		t.Fatal("unknown follow-up was queued or lost its draft")
	}
}

// TestKnownModelBoundSlashSubmission preserves templates and skills, including
// namespaced skills, without loading their contents during submission checks.
func TestKnownModelBoundSlashSubmission(t *testing.T) {
	for _, text := range []string{"/review args", "/skill:review args", "/skill:team/review args"} {
		m := &InteractiveMode{editor: tui.NewEditor(), slashRegistry: NewSlashRegistry()}
		m.promptTemplates = []PromptTemplate{{Name: "review"}}
		m.opts.Skills = []*SkillDef{{Name: "review"}, {Name: "team/review"}}
		m.setupEditorSubmitHandler(context.Background())
		var submitted string
		m.onInputCallback = func(text string) { submitted = text }
		m.editor.SetText(text)
		m.editor.HandleInput("\r")
		if submitted != text {
			t.Fatalf("known command %q was not submitted: %q", text, submitted)
		}
	}
}

// TestSlashMenuContinuesIntoArguments covers typing, Tab name completion, and
// accepting an option without submitting; Enter on a name keeps its old behavior.
func TestSlashMenuContinuesIntoArguments(t *testing.T) {
	for _, completeName := range []bool{false, true} {
		m := &InteractiveMode{}
		m.opts.CWD, m.opts.AgentDir = t.TempDir(), t.TempDir()
		editor := tui.NewEditor()
		editor.SetAutocomplete(m.buildAutocompleteProvider())
		submits := 0
		editor.OnSubmit = func(string) { submits++ }
		if completeName {
			editor.HandleInput("/subagents")
			editor.HandleInput("\t")
		} else {
			editor.HandleInput("/subagents ")
		}
		if editor.Text() != "/subagents " || !editor.AutocompleteOpen() {
			t.Fatalf("completeName=%t: no argument menu after %q", completeName, editor.Text())
		}
		editor.HandleInput("\t")
		if editor.Text() != "/subagents on" || editor.AutocompleteOpen() || submits != 0 {
			t.Fatalf("option acceptance: text=%q open=%t submits=%d", editor.Text(), editor.AutocompleteOpen(), submits)
		}
		editor.SetText("/subagents")
		editor.RefreshAutocomplete()
		editor.HandleInput("\r")
		if submits != 1 {
			t.Fatal("Enter on a name no longer submits")
		}
	}
}
