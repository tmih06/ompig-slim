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
		{"/usage ", "show,reset"},
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

// TestUnknownSlashCommandStaysInEditor verifies an unknown leading slash is
// rejected without losing the editable draft.
func TestUnknownSlashCommandStaysInEditor(t *testing.T) {
	m := &InteractiveMode{editor: tui.NewEditor(), slashRegistry: NewSlashRegistry()}
	m.setupEditorSubmitHandler(context.Background())
	m.editor.SetText("/does-not-exist")
	m.editor.HandleInput("\r")
	if got := m.editor.Text(); got != "/does-not-exist" {
		t.Fatalf("unknown slash command was submitted: editor text %q", got)
	}
}

// TestUnknownSlashCommandOnlyChecksLeadingText keeps slash-looking prose and
// loaded prompt resources outside the rejection path.
func TestUnknownSlashCommandOnlyChecksLeadingText(t *testing.T) {
	m := &InteractiveMode{slashRegistry: NewSlashRegistry(), promptTemplates: []PromptTemplate{{Name: "review"}}, opts: InteractiveModeOptions{Skills: []*SkillDef{{Name: "ship"}}}}
	for _, tc := range []struct {
		prompt  string
		unknown bool
	}{
		{"/missing", true},
		{"plain /missing", false},
		{"/review", false},
		{"/skill:ship", false},
		{"/quit extra", false},
	} {
		unknown, _ := m.unknownSlashCommand(tc.prompt)
		if unknown != tc.unknown {
			t.Errorf("unknownSlashCommand(%q) = %t, want %t", tc.prompt, unknown, tc.unknown)
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
