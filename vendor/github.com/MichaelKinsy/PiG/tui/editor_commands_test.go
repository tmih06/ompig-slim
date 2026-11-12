package tui

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestConfirmedCommandColors observes rendered editor rows after completion:
// commands have stable theme colors, arguments stay plain, and source is intact.
func TestConfirmedCommandColors(t *testing.T) {
	p := NewCombinedProvider([]SlashCommand{{Name: "goal"}, {Name: "model"}, {Name: "skill:review"}}, "", "")
	colors := map[string]string{"goal": "accent", "model": "warning"}
	for name, token := range colors {
		e := NewEditor()
		e.SetAutocomplete(p)
		e.SetText("/" + name)
		e.RefreshAutocomplete()
		e.HandleInput("\t")
		e.HandleInput("plain arguments")
		rendered := strings.Join(e.Render(80), "\n")
		want := ActiveTheme().GetFgAnsi(token) + "/" + name + "\x1b[39m"
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing colored command %q in %q", want, rendered)
		}
		if !strings.Contains(rendered, "plain arguments") || e.Text() != "/"+name+" plain arguments" {
			t.Fatalf("rendering changed source: %q", e.Text())
		}
		for _, width := range []int{6, 12, 80} {
			for _, row := range e.Render(width) {
				if widthx.VisibleWidth(row) > width {
					t.Fatalf("row wider than %d: %q", width, row)
				}
			}
		}
	}
}

// TestCommandColorPreservesLayout keeps cursor markers, badges and wrapping
// byte-identical apart from foreground escapes, including a cursor in the name.
func TestCommandColorPreservesLayout(t *testing.T) {
	for _, text := range []string{"/goal /skill:review next", "  /model plain", "/model\nsecond line", "/unknown plain", "/mod", "/skill:review next"} {
		for _, width := range []int{4, 8, 40} {
			for _, cursor := range []int{0, 3, len(strings.Split(text, "\n")[0])} {
				colored, plain := NewEditor(), NewEditor()
				colored.SetAutocomplete(NewSlashOnlyProvider([]SlashCommand{{Name: "goal"}, {Name: "model"}, {Name: "skill:review"}}))
				plain.SetAutocomplete(NewSlashOnlyProvider([]SlashCommand{{Name: "skill:review"}}))
				for _, e := range []*Editor{colored, plain} {
					e.SetText(text)
					e.Focused = true
					e.cursor = [2]int{0, cursor}
				}
				got, want := strings.Join(colored.Render(width), "\n"), strings.Join(plain.Render(width), "\n")
				got, want = stripCommandTestSGR(got), stripCommandTestSGR(want)
				if got != want {
					t.Fatalf("%q width %d cursor %d changed layout:\ngot  %q\nwant %q", text, width, cursor, got, want)
				}
			}
		}
	}
}

// stripCommandTestSGR removes only styling, retaining APC cursor/fake-cursor
// markers so duplicated or dropped markers cannot hide behind ANSI stripping.
func stripCommandTestSGR(text string) string {
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], "\x1b[") {
			if n := widthx.ExtractAnsi(text, i); n > 0 {
				text = text[:i] + text[i+n:]
				continue
			}
		}
		i++
	}
	return text
}
