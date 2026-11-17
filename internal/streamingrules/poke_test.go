package streamingrules

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
)

func assertPoke(t *testing.T, api *captureAPI, draft extension.SessionBoundaryDraft, tokens ...string) {
	t.Helper()
	if draft.Type != extension.SessionBoundaryDraftCustom || draft.CustomType != pokeType || api.renderer == nil {
		t.Fatal("poke is not a display-only rendered entry", draft)
	}
	raw, err := json.Marshal(draft.Data)
	if err != nil {
		t.Fatal(err)
	}
	var restored any
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	for _, data := range []any{draft.Data, restored} {
		component := api.renderer(extension.CustomEntry{CustomType: pokeType, Data: data}, extension.EntryRenderOptions{}, nil)
		if component == nil {
			t.Fatal("poke did not render")
		}
		text := strings.Join(component.Render(160), "\n")
		for _, token := range append([]string{"◇ Rule poke"}, tokens...) {
			if !strings.Contains(text, token) {
				t.Fatalf("missing %q in %q", token, text)
			}
		}
		for _, token := range []string{"system-reminder", "rule_violation", "Streaming rules sent a reminder", "Streaming rules blocked"} {
			if strings.Contains(text, token) {
				t.Fatalf("internal or warning text leaked into poke: %q", text)
			}
		}
		for _, width := range []int{20, 40, 80} {
			if len(component.Render(width)) == 0 {
				t.Fatal("poke disappeared at narrow width", width)
			}
		}
	}
}

func TestPatternSummary(t *testing.T) {
	for _, test := range []struct {
		description, body, want string
	}{
		{"Deprecated import.", "Full correction.", "Deprecated import."},
		{"  A\n concise\tpattern. ", "Full correction.", "A concise pattern."},
		{"", "Use GOOD instead.\n\nLong instructions.", "Use GOOD instead."},
		{"", strings.Repeat("界", 200), strings.Repeat("界", 157) + "..."},
	} {
		if got := summarizePattern(test.description, test.body); got != test.want {
			t.Fatalf("summary=%q want=%q", got, test.want)
		}
	}
	rule, _, err := parseRule("custom", "custom.md", "---\ncondition: BAD\ndescription: Avoid debug output.\n---\nLong instructions.", false)
	if err != nil || rule.summary != "Avoid debug output." {
		t.Fatal(rule, err)
	}
}

func TestPokeMultipleRules(t *testing.T) {
	api := &captureAPI{renderer: renderPoke}
	draft := pokeDraft([]*rule{
		{name: "first", summary: "First pattern."},
		{name: "second", summary: "Second pattern."},
	}, false)
	assertPoke(t, api, draft, "Reminder sent", "first", "First pattern.", "second", "Second pattern.")
}
