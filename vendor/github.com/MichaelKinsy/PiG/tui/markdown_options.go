package tui

// Ports packages/tui/src/components/markdown.ts (constructor options and text replacement).

// MarkdownTheme supplies the styling functions used by Markdown. It mirrors packages/tui/src/components/markdown.ts,
// plus the members OMP uses for its colorful Markdown palette:
//
//   - HeadingAt styles a heading of a given depth (1-6). Heading is HeadingAt(1) and remains the field every other caller
//     reads, so a theme that sets only Heading still styles every level.
//   - TableHeader styles a table's header row. Nil keeps the upstream bold-only header.
//
// A nil HeadingAt falls back to Heading, and both nil fall back to unstyled text.
type MarkdownTheme struct {
	Heading, Link, LinkUrl, Code, CodeBlock, CodeBlockBorder func(string) string
	Quote, QuoteBorder, Hr, ListBullet                       func(string) string
	Bold, Italic, Strikethrough, Underline                   func(string) string
	// HeadingAt styles a heading by depth; nil means Heading for every depth.
	HeadingAt func(text string, depth int) string
	// TableHeader styles a table's header cells, which get Bold when nil.
	TableHeader     func(string) string
	HighlightCode   func(code, lang string) []string
	CodeBlockIndent *string
}

// heading returns the style for a heading of the given depth: HeadingAt when the theme names one, Heading otherwise, and the
// identity for a theme that names neither. depth is one-based, as marked reports it.
func (theme MarkdownTheme) heading(depth int) func(string) string {
	if theme.HeadingAt != nil {
		return func(text string) string { return theme.HeadingAt(text, depth) }
	}
	if theme.Heading != nil {
		return theme.Heading
	}
	return func(text string) string { return text }
}

// tableHeader returns the style for a table header cell, Bold for a theme without one.
func (theme MarkdownTheme) tableHeader() func(string) string {
	if theme.TableHeader != nil {
		return theme.TableHeader
	}
	if theme.Bold != nil {
		return theme.Bold
	}
	return func(text string) string { return text }
}

// DefaultTextStyle decorates ordinary Markdown text; background is applied after layout and padding.
type DefaultTextStyle struct {
	Color, BgColor                         func(string) string
	Bold, Italic, Strikethrough, Underline bool
}

// MarkdownOptions controls source preservation, display transforms and math rendering. Nil RenderLatex selects the upstream default, true.
type MarkdownOptions struct {
	PreserveOrderedListMarkers bool
	PreserveBackslashEscapes   bool
	Transform                  func(markdown string, availableWidth int) string
	RenderLatex                *bool
}

// NewMarkdownWithOptions maps the full upstream constructor. NewMarkdown supplies its normal unpadded, active-theme defaults without a Go overload.
func NewMarkdownWithOptions(content string, paddingX, paddingY int, theme *MarkdownTheme, style *DefaultTextStyle, options *MarkdownOptions) *Markdown {
	m := &Markdown{Content: content, paddingX: paddingX, paddingY: paddingY, theme: theme, defaultTextStyle: style}
	if options != nil {
		m.options = *options
		m.Transform = options.Transform
		if options.RenderLatex != nil {
			m.options.RenderLatex = new(*options.RenderLatex)
		}
	}
	return m
}

// SetText replaces source text and invalidates its rendered output.
func (m *Markdown) SetText(text string) { m.Content = text; m.textChanged() }
