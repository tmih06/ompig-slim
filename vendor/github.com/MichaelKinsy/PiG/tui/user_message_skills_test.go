package tui

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestSubmittedSkillsMatchComposer checks style, positions, repeat invocations,
// wrapping and disclosure without a live skill registry or separate cards.
func TestSubmittedSkillsMatchComposer(t *testing.T) {
	for _, name := range []string{"review", "team/check", "foo_bar_baz"} {
		t.Run(name, func(t *testing.T) {
			source := "please /skill:" + name + " fix /skill:" + name + " now /skill:unknown"
			u := NewUserMessageComponent(source, nil, 1, nil, "", "")
			u.SetSkills([]ParsedSkillBlock{{Name: name, Content: "SECRET BODY"}})
			transforms := 0
			u.SetMarkdownTransform(func(text string, _ int) string {
				transforms++
				if text != source {
					t.Fatalf("transform received styled text rather than source: %q", text)
				}
				return text
			})
			editor := NewEditor()
			editor.SetAutocomplete(NewSlashOnlyProvider([]SlashCommand{{Name: "skill:" + name}}))
			editor.SetText(source)
			got := strings.Join(u.Render(120), "\n")
			if again := strings.Join(u.Render(120), "\n"); again != got || transforms != 1 {
				t.Fatal("unchanged message was repainted or transformed again")
			}
			want := widthx.StripAnsi(editor.skillDisplay(source, -1, false))
			if !strings.Contains(widthx.StripAnsi(got), want) {
				t.Fatalf("submitted prompt differs from composer: %q; want %q", got, want)
			}
			if strings.Count(got, skillBadge(name)) != 2 {
				t.Fatalf("submitted badges lost composer styles: %q", got)
			}
			if strings.Contains(got, "SECRET BODY") {
				t.Fatal("collapsed submission exposed a skill body")
			}
			for _, row := range u.Render(18) {
				if widthx.VisibleWidth(row) > 18 {
					t.Fatalf("badge exceeded transcript width: %q", row)
				}
			}
			u.SetExpanded(true)
			if !strings.Contains(widthx.StripAnsi(strings.Join(u.Render(120), "\n")), "SECRET BODY") {
				t.Fatal("global expand did not disclose skill body")
			}
			u.SetExpanded(false)
			if strings.Contains(strings.Join(u.Render(120), "\n"), "SECRET BODY") {
				t.Fatal("global collapse left skill body visible")
			}
		})
	}
}
