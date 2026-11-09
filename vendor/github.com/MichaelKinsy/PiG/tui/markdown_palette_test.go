package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// swapTheme installs theme for the duration of a test and restores the active
// theme afterwards. Selecting a theme is process-wide state, so a test that
// forgot to restore it would leak into the next one.
func swapTheme(t *testing.T, theme *Theme) {
	t.Helper()
	previous := ActiveTheme()
	t.Cleanup(func() { SetThemeInstance(previous) })
	SetThemeInstance(theme)
}

// paletteHueTheme is a theme whose tokens all alias one color, which is how the
// palettes that motivated the palette roles look (Dracula's mdCode is its
// magenta), plus a terminal.ansi palette with a distinct slot per index.
func paletteHueTheme(t *testing.T) *Theme {
	t.Helper()
	var foregrounds, backgrounds []ThemeTokenValue
	for _, token := range themeColorTokens {
		value := ThemeColorValue{Text: "#808080", isSet: true}
		if themeBackgroundTokens[token.name] {
			backgrounds = append(backgrounds, ThemeTokenValue{Token: token.name, Value: ThemeColorValue{Text: "#101010", isSet: true}})
			continue
		}
		foregrounds = append(foregrounds, ThemeTokenValue{Token: token.name, Value: value})
	}
	theme, err := NewTheme(foregrounds, backgrounds, TerminalColorModeTrueColor, ThemeOptions{Name: "palette-test"})
	if err != nil {
		t.Fatalf("build theme: %v", err)
	}
	theme.ansiFg = make([]string, ANSISlotCount)
	for slot := range ANSISlotCount {
		theme.ansiFg[slot] = "\x1b[38;5;" + strconv.Itoa(240+slot) + "m"
	}
	theme.populateFields()
	return theme
}

// TestMarkdownPaletteHues covers the roles Markdown paints from a theme's ANSI
// palette: a distinct hue per heading depth, emphasis and inline code in their
// own slots, and a colored table header.
func TestMarkdownPaletteHues(t *testing.T) {
	theme := paletteHueTheme(t)
	swapTheme(t, theme)

	source := "## Two\n\n### Three\n\n**bold** *italic* `code`\n\n| h |\n|---|\n| 1 |\n"
	rendered := strings.Join(NewMarkdown(source).Render(40), "\n")
	for _, want := range []struct {
		name, slot string
	}{
		{"H2", theme.ansiFg[ANSIYellow]},
		{"H3", theme.ansiFg[ANSIBlue]},
		{"bold and italic", theme.ansiFg[ANSIMagenta]},
		{"inline code", theme.ansiFg[ANSIGreen]},
		{"table header", theme.ansiFg[ANSIYellow]},
	} {
		if !strings.Contains(rendered, want.slot) {
			t.Fatalf("%s is not drawn in its palette slot %q:\n%s", want.name, want.slot, rendered)
		}
	}
	if strings.Contains(rendered, theme.ansiFg[ANSIMagenta]+"Three") {
		t.Fatalf("H3 took H1's hue")
	}
}

// TestMarkdownFallsBackToTokens covers a theme without a palette: the roles keep
// the theme's md* tokens and plain emphasis, which is the rendering before the
// palette roles existed.
func TestMarkdownFallsBackToTokens(t *testing.T) {
	theme := paletteHueTheme(t)
	theme.ansiFg = nil
	swapTheme(t, theme)

	rendered := strings.Join(NewMarkdown("## Two\n\n**bold** `code`\n").Render(40), "\n")
	for slot := range ANSISlotCount {
		if strings.Contains(rendered, theme.ansiFgDefaultForTest(slot)) {
			t.Fatalf("slot %d is painted for a theme without a palette:\n%q", slot, rendered)
		}
	}
	if !strings.Contains(rendered, theme.MDHeading) || !strings.Contains(rendered, theme.MDCode) {
		t.Fatalf("tokens were not used:\n%q", rendered)
	}
}

// ansiFgDefaultForTest is the terminal-palette fallback ANSIFg paints for slot,
// used only to prove a palette slot was not chosen.
func (t *Theme) ansiFgDefaultForTest(slot int) string {
	return t.ANSIFg(slot, "")
}

// TestMarkdownHeadingKeepsOwnHue covers a heading's own emphasis: a `**bold**`
// run inside a heading takes the heading's hue rather than the body prose's
// emphasis hue, because a heading is already the strongest text on its row.
func TestMarkdownHeadingKeepsOwnHue(t *testing.T) {
	theme := paletteHueTheme(t)
	swapTheme(t, theme)

	rendered := strings.Join(NewMarkdown("## a **b** c\n").Render(40), "\n")
	if strings.Contains(rendered, theme.ansiFg[ANSIMagenta]+"\x1b[1mb") {
		t.Fatalf("heading emphasis took the body hue:\n%q", rendered)
	}
	if !strings.Contains(rendered, theme.ansiFg[ANSIYellow]) {
		t.Fatalf("heading lost its own hue:\n%q", rendered)
	}
}

// TestMarkdownPaletteKeepsLayout covers the invariant that colors do not change
// layout: the visible width and the stripped text of every row are identical
// with and without a palette, because only a theme switch differs.
func TestMarkdownPaletteKeepsLayout(t *testing.T) {
	source := "## Heading\n\nSome **bold**, *italic*, `code` and [link](https://x.y).\n\n- bullet\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"

	plain := NewMarkdown(source)
	plainTheme := paletteHueTheme(t)
	plainTheme.ansiFg = nil
	swapTheme(t, plainTheme)
	plainLines := plain.Render(60)

	colored := NewMarkdown(source)
	swapTheme(t, paletteHueTheme(t))
	coloredLines := colored.Render(60)

	if len(plainLines) != len(coloredLines) {
		t.Fatalf("row count changed: %d vs %d", len(plainLines), len(coloredLines))
	}
	for i := range plainLines {
		if widthx.VisibleWidth(plainLines[i]) != widthx.VisibleWidth(coloredLines[i]) {
			t.Fatalf("row %d width changed: %d vs %d", i, widthx.VisibleWidth(plainLines[i]), widthx.VisibleWidth(coloredLines[i]))
		}
		if widthx.StripAnsi(plainLines[i]) != widthx.StripAnsi(coloredLines[i]) {
			t.Fatalf("row %d text changed:\n%q\n%q", i, plainLines[i], coloredLines[i])
		}
	}
}
