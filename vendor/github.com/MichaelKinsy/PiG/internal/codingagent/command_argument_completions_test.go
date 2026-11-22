package codingagent

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
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
	texts := []string{"/tmp/project/file", "/tmp/project/file explain this", "/new/file.go", "/file.txt explain this", "/", "./file.go explain this", "~/file.go", "C:\\project\\file.go", "explain /not-a-command"}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	volumeRoot := filepath.VolumeName(cwd) + string(filepath.Separator)
	relativeCWD, err := filepath.Rel(volumeRoot, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if relativeCWD != "." {
		rootDirectory := "/" + strings.Split(relativeCWD, string(filepath.Separator))[0]
		if info, err := os.Stat(rootDirectory); err == nil && info.IsDir() {
			texts = append(texts, rootDirectory)
		}
	}
	for _, text := range texts {
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

// TestSlashKeywordSuggestionsInHostWiring observes the built provider: a
// purpose word ranks the matching builtin first and its row shows the keyword
// tag, while the completed text keeps only the command name.
func TestSlashKeywordSuggestionsInHostWiring(t *testing.T) {
	m := &InteractiveMode{}
	m.opts.CWD, m.opts.AgentDir = t.TempDir(), t.TempDir()
	provider := m.buildAutocompleteProvider()
	result := provider.GetSuggestions(context.Background(), []string{"/clea"}, 0, len("/clea"), tui.AutocompleteSuggestionOptions{})
	if result == nil || len(result.Items) == 0 || result.Items[0].Value != "new" {
		t.Fatalf("/clea did not rank /new first: %+v", result)
	}
	if result.Items[0].Label != "new (clear, fresh)" {
		t.Fatalf("keyword tag missing from label: %q", result.Items[0].Label)
	}
	lines, row, col := provider.ApplyCompletion([]string{"/clea"}, 0, len("/clea"), result.Items[0], result.Prefix)
	if lines[row] != "/new " || row != 0 || col != len("/new ") {
		t.Fatalf("completion inserted %q @%d,%d", lines[row], row, col)
	}
}

// TestUnknownSlashSuggestsClosestCommand observes the rejected submission:
// the error names the closest offered command, including through a keyword,
// while unrelated input gets no suggestion and the draft stays editable.
func TestUnknownSlashSuggestsClosestCommand(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"/clea", "Error: Unknown slash command: /clea. Did you mean /new?"},
		{"/clear", "Error: Unknown slash command: /clear. Did you mean /new?"},
		{"/exit", "Error: Unknown slash command: /exit. Did you mean /quit?"},
		{"/neww", "Error: Unknown slash command: /neww. Did you mean /new?"},
		{"/seeting", "Error: Unknown slash command: /seeting. Did you mean /settings?"},
		{"/zzzzz", "Error: Unknown slash command: /zzzzz"},
	} {
		m := &InteractiveMode{editor: tui.NewEditor(), slashRegistry: NewSlashRegistry(),
			chatContainer: tui.NewContainer(), tuiInst: tui.NewWithOutput(io.Discard, 80, 24)}
		m.setupEditorSubmitHandler(context.Background())
		m.editor.SetText(tc.text)
		m.editor.HandleInput("\r")
		transcript := widthx.StripAnsi(strings.Join(m.chatContainer.Render(100), "\n"))
		if !strings.Contains(transcript, tc.want) {
			t.Fatalf("%q missing %q in transcript %q", tc.text, tc.want, transcript)
		}
		if m.editor.Text() != tc.text {
			t.Fatalf("%q rejected draft lost: %q", tc.text, m.editor.Text())
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
