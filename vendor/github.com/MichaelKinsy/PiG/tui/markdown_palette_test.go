package tui

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

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
	// H3 carries the blue slot, not H1's magenta. The heading's own emphasis and its
	// `###` prefix put SGR codes between the slot and the word, so find the row by
	// its visible text and check the codes on that row.
	found := false
	for _, line := range strings.Split(rendered, "\n") {
		if !strings.Contains(widthx.StripAnsi(line), "Three") {
			continue
		}
		found = true
		if !strings.Contains(line, theme.ansiFg[ANSIBlue]) {
			t.Fatalf("H3 is not drawn in the blue slot: %q", line)
		}
		if strings.Contains(line, theme.ansiFg[ANSIMagenta]) {
			t.Fatalf("H3 took H1's magenta hue: %q", line)
		}
	}
	if !found {
		t.Fatalf("H3 row not found in:\n%s", rendered)
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
	// With no palette, no ANSI slot may appear. Compare against the sequence the
	// renderer would emit for a slot, not a bare ANSIFg prefix, so the search string
	// can actually occur in rendered output.
	for slot := range ANSISlotCount {
		if open := paletteSlotAnsi(theme, slot); open != "" && strings.Contains(rendered, open) {
			t.Fatalf("slot %d is painted for a theme without a palette:\n%q", slot, rendered)
		}
	}
	if !strings.Contains(rendered, theme.MDHeading) || !strings.Contains(rendered, theme.MDCode) {
		t.Fatalf("tokens were not used:\n%q", rendered)
	}
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

// TestProseNoDuplicatePrefix covers the escape-byte de-duplication in the
// sequential inline renderer: a text run that follows an inline token reuses the
// opening codes that token left pending instead of writing them again.
//
// The risk is asymmetric. Writing the codes twice is invisible; reusing a prefix
// that is not actually there drops the run's color, which is a real regression.
// Adjacent runs are what tests it: escaped punctuation and inline HTML each render
// as their own run, and a run ending in closing codes must not leave the flag
// raised for the next one. So each case asserts that no prose run was lost, and
// that the same codes were not written back to back.
func TestProseNoDuplicatePrefix(t *testing.T) {
	theme := paletteHueTheme(t)
	swapTheme(t, theme)
	prose := theme.ProseFg()
	if prose == "" {
		t.Fatal("fixture theme has no prose color")
	}

	for _, test := range []struct{ name, source string }{
		{"text after emphasis", "a **b** c\n"},
		{"text after a code span", "a `b` c\n"},
		{"adjacent emphasis", "a **b****c** d\n"},
		{"adjacent code spans", "a `b``c` d\n"},
		{"escaped punctuation", "a\\*b\n"},
		{"escape mid-word", "text\\*more\n"},
		{"emphasis then escapes", "a *b* \\*c\\* d\n"},
		{"inline HTML", "a <!--h--> b\n"},
		{"emphasis then HTML", "a **b** <!--h--> c\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The prose color reaches a block through SetDefaultColor, the same seam
			// AssistantMessageComponent and proseMarkdown use.
			block := NewMarkdown(test.source)
			block.SetDefaultColor(prose)
			rendered := strings.Join(block.Render(60), "\n")
			if strings.Contains(rendered, prose+prose) {
				t.Fatalf("opening codes written twice: %q", rendered)
			}
			// The regression this guards is a prose run rendered with no foreground at
			// all. Every run of non-space text must be inside one, which is exactly the
			// case escaped punctuation and inline HTML used to break.
			if outside := glyphsOutsideForeground(rendered); outside != "" {
				t.Fatalf("glyphs outside a foreground run: %q in %q", outside, rendered)
			}
		})
	}
}

// glyphsOutsideForeground returns the non-space glyphs that sit outside any
// foreground run.
func glyphsOutsideForeground(rendered string) string {
	var outside strings.Builder
	open := false
	for rest := rendered; rest != ""; {
		if rest[0] == '\x1b' {
			next := sgrEnd(rest)
			if next == 0 {
				rest = rest[1:]
				continue
			}
			code := rest[:next]
			switch {
			case strings.HasPrefix(code, "\x1b[38;"):
				open = true
			case code == "\x1b[39m" || code == "\x1b[0m":
				open = false
			}
			rest = rest[next:]
			continue
		}
		_, size := utf8.DecodeRuneInString(rest)
		if ch := rest[:size]; ch != " " && !open {
			outside.WriteString(ch)
		}
		rest = rest[size:]
	}
	return outside.String()
}

// sgrEnd is the byte length of the escape sequence starting at s, or 0 when s does
// not begin one.
func sgrEnd(s string) int {
	if len(s) < 2 || s[0] != '\x1b' || s[1] != '[' {
		return 0
	}
	for i := 2; i < len(s); i++ {
		if s[i] == 'm' {
			return i + 1
		}
	}
	return 0
}

// TestProseColorFollowsThemeSwitch covers a reply that outlives its theme: the block
// is built under one theme and rendered under another, and its prose must carry the
// active theme's color. Resolving the color once at construction left the old escape
// on screen until the message was rebuilt.
func TestProseColorFollowsThemeSwitch(t *testing.T) {
	titanium := optionalPalette(t, "titanium")
	light, err := loadBuiltinThemeWithMode("light", TerminalColorModeTrueColor)
	if err != nil {
		t.Fatalf("light: %v", err)
	}
	if titanium.ProseFg() == light.ProseFg() {
		t.Fatal("fixtures share a prose color; the test would prove nothing")
	}

	previous := ActiveTheme()
	previousRegistry := ActiveThemeRegistry()
	t.Cleanup(func() {
		SetThemeRegistry(previousRegistry)
		SetThemeInstance(previous)
	})
	SetThemeRegistry(NewThemeRegistry())
	SetThemeInstance(titanium)

	block := NewAssistantMessageComponent(&AssistantMessage{
		Content: []AssistantContentBlock{{Type: "text", Text: "hello\n"}},
	}, true, nil, "", nil, nil, "", "")

	SetThemeInstance(light)
	rendered := strings.Join(block.Render(60), "\n")
	if !strings.Contains(rendered, light.ProseFg()) {
		t.Fatalf("prose did not follow the theme switch, want %q in %q", light.ProseFg(), rendered)
	}
	if strings.Contains(rendered, titanium.ProseFg()) {
		t.Fatalf("prose kept the theme it was built under (%q): %q", titanium.ProseFg(), rendered)
	}
}
