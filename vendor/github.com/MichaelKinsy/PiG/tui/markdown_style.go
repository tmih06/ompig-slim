package tui

// Ports packages/tui/src/components/markdown.ts (style contexts and invalidation).

import (
	"strings"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func markdownForeground(open, text string) string {
	if open == "" {
		return text
	}
	return open + text + FgClose(open)
}
func markdownDecoration(open, close, text string) string {
	if text == "" || chalkModifiersOff.Load() {
		return text
	}
	return ansiSpan(open, close, text)
}

// markdownBold, markdownItalic and markdownStrikethrough are the emphasis styles of every built-in markdown theme. The themes share these functions, so the one-pass span renderer recognizes them by function identity.
func markdownBold(s string) string { return markdownDecoration("\x1b[1m", SGRBoldDimReset, s) }

func markdownItalic(s string) string { return markdownDecoration("\x1b[3m", SGRItalicReset, s) }

func markdownStrikethrough(s string) string {
	return markdownDecoration("\x1b[9m", SGRStrikeReset, s)
}

// colorfulHeadingSlots are the ANSI palette slots colorful Markdown paints headings with, by depth: H1 magenta,
// H2 yellow, H3 and deeper blue. Upstream's COLORFUL_HEADING_COLORS.
var colorfulHeadingSlots = [3]int{ANSIMagenta, ANSIYellow, ANSIBlue}

// paletteRole builds one of colorful Markdown's roles: paint in ANSI palette slot when the active theme names a palette,
// else fall back to token. An empty text stays empty, so a role never emits escape sequences around nothing.
//
// A role is a closure over constants that reads ActiveTheme() when called, never a captured theme. Theme switches must
// restyle components that already hold a MarkdownTheme, and the render cache identifies a theme by its style functions'
// code addresses, which only stay fixed while each role is one closure at one source location.
func paletteRole(slot int, token string) func(string) string {
	return func(s string) string {
		if s == "" {
			return s
		}
		th := ActiveTheme()
		if th.HasANSIPalette() {
			return th.ANSIFg(slot, s)
		}
		if token == "" {
			// No token to fall back to: the role's caller supplies the whole style.
			return s
		}
		return markdownForeground(th.GetFgAnsi(token), s)
	}
}

// colorfulHeading is the heading role for a depth: H1 magenta, H2 yellow, H3 and deeper blue, the one palette slot set
// upstream's COLORFUL_HEADING_COLORS uses. Headings come from the palette rather than mdHeading because themes commonly
// alias several md* tokens to one color (Dracula's mdCode is its magenta), which would collapse every level into one hue.
func colorfulHeading(depth int) func(string) string {
	if depth <= 0 {
		depth = 1
	}
	return paletteRole(colorfulHeadingSlots[min(depth, len(colorfulHeadingSlots))-1], "mdHeading")
}

// decoratedRole is paletteRole around a text attribute: decorate applies bold, italic or both, then the hue.
// fallbackToken is the md* token a theme without a palette uses; "" means decorate alone, the pre-palette rendering.
func decoratedRole(slot int, fallbackToken string, decorate func(string) string) func(string) string {
	tinted := paletteRole(slot, fallbackToken)
	return func(s string) string {
		if s == "" {
			return s
		}
		return tinted(decorate(s))
	}
}

// GetMarkdownTheme returns the markdown theme that styles each span with the active theme's markdown tokens (upstream theme.ts getMarkdownTheme). Every function reads ActiveTheme when it runs, so a theme switch restyles components that hold the result.
//
// When the theme carries an ANSI palette, headings, emphasis, inline code and table headers take their hues from that palette, as upstream's colorfulMarkdownRoles do: H1 magenta, H2 yellow, H3+ blue, strong/emphasis magenta, inline code green, table headers yellow. All hues come from one palette so the roles stay distinct; a theme without a palette keeps its md* tokens, which is the presentation before the palette roles existed.
func GetMarkdownTheme() MarkdownTheme {
	return MarkdownTheme{
		Heading:         func(s string) string { return markdownForeground(ActiveTheme().MDHeading, s) },
		HeadingAt:       func(s string, depth int) string { return colorfulHeading(depth)(s) },
		Link:            func(s string) string { return markdownForeground(ActiveTheme().MDLink, s) },
		LinkUrl:         func(s string) string { return markdownForeground(ActiveTheme().MDLinkUrl, s) },
		Code:            colorfulInlineCode,
		CodeBlock:       func(s string) string { return markdownForeground(ActiveTheme().MDCodeBlock, s) },
		CodeBlockBorder: func(s string) string { return markdownForeground(ActiveTheme().MDCodeBlockBorder, s) },
		Quote:           func(s string) string { return markdownForeground(ActiveTheme().MDQuote, s) },
		QuoteBorder:     func(s string) string { return markdownForeground(ActiveTheme().MDQuoteBorder, s) },
		Hr:              func(s string) string { return markdownForeground(ActiveTheme().MDHr, s) },
		ListBullet:      func(s string) string { return markdownForeground(ActiveTheme().MDListBullet, s) },
		Bold:            decoratedRole(ANSIMagenta, "", markdownBold),
		Italic:          decoratedRole(ANSIMagenta, "", markdownItalic),
		Strikethrough:   markdownStrikethrough,
		Underline:       func(s string) string { return markdownDecoration("\x1b[4m", SGRUnderlineReset, s) },
		TableHeader:     decoratedRole(ANSIYellow, "mdHeading", markdownBold),
		HighlightCode:   HighlightCode,
	}
}

// colorfulInlineCode is the inline-code role: the palette's green, or mdCode for a theme without a palette.
func colorfulInlineCode(s string) string { return paletteRole(ANSIGreen, "mdCode")(s) }

// activeMarkdownTheme is GetMarkdownTheme built once. Its functions read the active theme when called, so one value serves every render.
var activeMarkdownTheme = GetMarkdownTheme()

func (m *Markdown) markdownTheme() MarkdownTheme {
	if m.theme != nil {
		return *m.theme
	}
	return activeMarkdownTheme
}
func markdownStylePrefix(style func(string) string) string {
	styled := style("\x00")
	before, _, ok := strings.Cut(styled, "\x00")
	if !ok {
		return ""
	}
	return before
}
func (m *Markdown) defaultInlineStyleContext() inlineStyleContext {
	if m.styleContext != nil {
		return *m.styleContext
	}
	if m.styleOwner != nil {
		return m.styleOwner.defaultInlineStyleContext()
	}
	if !m.hasDefaultStylePrefix {
		m.defaultStylePrefix = markdownStylePrefix(m.applyDefaultStyle)
		m.hasDefaultStylePrefix = true
	}
	return inlineStyleContext{applyText: m.applyDefaultStyle, stylePrefix: m.defaultStylePrefix, identity: m.defaultStyleIsIdentity()}
}

// defaultStyleIsIdentity reports whether applyDefaultStyle leaves text unchanged.
func (m *Markdown) defaultStyleIsIdentity() bool {
	style := m.defaultTextStyle
	colored := style != nil && style.Color != nil
	if m.defaultColorSet {
		colored = m.defaultColor != ""
	}
	return !colored && !m.defaultItalic && (style == nil || !style.Bold && !style.Italic && !style.Strikethrough && !style.Underline)
}
func (m *Markdown) childMarkdown(content string) *Markdown {
	child := NewMarkdownWithOptions(content, 0, 0, m.theme, m.defaultTextStyle, &m.options)
	child.Transform = nil
	child.defaultColor, child.defaultColorSet = m.defaultColor, m.defaultColorSet
	child.defaultItalic = m.defaultItalic
	child.styleContext = m.styleContext
	child.styleOwner = m
	child.inlineState = m.lexerState()
	if m.styleOwner != nil {
		child.styleOwner = m.styleOwner
	}
	return child
}
func (m *Markdown) latexEnabled() bool { return m.options.RenderLatex == nil || *m.options.RenderLatex }

// renderQuote returns the quote's rows and whether it has a border: a quote narrower than three columns has none, and its own spacing row is not added (Pi 8a2ebd385).
func (m *Markdown) renderQuote(content string, lazyUnderlines, lazyLines, lazyIndented, lazyBreaks []int, width int, notLast bool) ([]string, bool) {
	theme := m.markdownTheme()
	quoteStyle := func(text string) string { return theme.Quote(theme.Italic(text)) }
	prefix := markdownStylePrefix(quoteStyle)
	child := m.childMarkdown(content)
	child.styleContext = &inlineStyleContext{applyText: func(s string) string { return s }, stylePrefix: prefix}
	child.lazyUnderlines = lazyUnderlines
	child.lazyLines = lazyLines
	child.lazyIndented = lazyIndented
	child.lazyBreaks = lazyBreaks
	child.keepPartialFence = m.keepPartialFence || notLast
	// upstream: markdown.ts renderToken blockquote (Pi 8a2ebd385): with no room for another border ("│ " plus one content column) the nested content renders
	// without one and without wrapping it again. Otherwise each extra level re-wraps every child line at width 1 and the output grows exponentially with the nesting depth.
	if width < 3 {
		// A quote of only whitespace has one space token, which renders as an empty row (markdown.ts case "space").
		if content != "" && strings.Trim(content, " \t\n") == "" {
			return []string{""}, false
		}
		child.styleContext = m.styleContext
		return child.renderContent(content, width), false
	}
	contentWidth := width - 2
	logical := child.renderContent(content, contentWidth)
	// markdown.ts drops trailing empty quote lines, which renderContent keeps only before a link definition.
	for len(logical) > 0 && logical[len(logical)-1] == "" {
		logical = logical[:len(logical)-1]
	}
	var out []string
	for _, line := range logical {
		if prefix != "" {
			line = strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+prefix)
		}
		for _, wrapped := range widthx.WrapTextWithAnsi(quoteStyle(line), contentWidth) {
			out = append(out, theme.QuoteBorder("│ ")+wrapped)
		}
	}
	return out, true
}
