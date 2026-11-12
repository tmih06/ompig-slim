package tui

import (
	"strings"
	"unicode"

	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// IsKnownCommand reports whether name belongs to this provider's slash registry.
func (p *CombinedProvider) IsKnownCommand(name string) bool { return p.slash.IsKnownCommand(name) }

// IsKnownCommand reports whether name belongs to this provider's slash registry.
func (p *SlashOnlyProvider) IsKnownCommand(name string) bool {
	for _, command := range p.Commands {
		if command.commandName() == name {
			return true
		}
	}
	return false
}

// colorCommandLines decorates only a completed leading command. It runs after
// layout and cursor/badge rendering so ANSI never enters source columns or undo.
// Registry order must not affect color, and theme tokens keep custom themes usable.
func (e *Editor) colorCommandLines(out []string, lines []layoutLine) {
	if len(e.lines) == 0 {
		return
	}
	text := e.lines[0]
	trimmed := widthx.JSTrimStart(text)
	if !strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "/skill:") {
		return
	}
	end := strings.IndexFunc(trimmed, unicode.IsSpace)
	if end < 0 {
		// No separator yet: the user is still completing the name.
		return
	}
	name := trimmed[1:end]
	known, ok := e.autocomplete.(interface{ IsKnownCommand(string) bool })
	if !ok || !known.IsKnownCommand(name) {
		return
	}
	palette := [...]string{"success", "warning", "accent"}
	color := 0
	for _, r := range name {
		color = (color + int(r)) % len(palette)
	}
	opening := ActiveTheme().GetFgAnsi(palette[color])
	start := jsstring.Length(text[:len(text)-len(trimmed)])
	stop := start + jsstring.Length(trimmed[:end])
	for i, line := range lines {
		if line.sourceLine != 0 {
			break
		}
		left, right := max(start-line.sourceStart, 0), min(stop-line.sourceStart, jsstring.Length(line.text))
		if left >= right {
			continue
		}
		from := widthx.VisibleWidth(jsstring.Slice(line.text, 0, left))
		to := widthx.VisibleWidth(jsstring.Slice(line.text, 0, right))
		out[i] = colorCommandRange(out[i], from, to, opening)
	}
}

// colorCommandRange inserts foreground escapes at cell boundaries without
// slicing/replaying ANSI: zero-width cursor markers must occur exactly once.
func colorCommandRange(row string, from, to int, opening string) string {
	start, offset, column := 0, 0, 0
	for offset < len(row) {
		if column == from {
			start = offset
			from = -1
		}
		if column == to {
			break
		}
		if n := widthx.ExtractAnsi(row, offset); n > 0 {
			offset += n
			continue
		}
		cluster, _ := widthx.FirstGrapheme(row[offset:])
		offset += len(cluster)
		column += widthx.GraphemeWidth(cluster)
	}
	return row[:start] + opening + row[start:offset] + "\x1b[39m" + row[offset:]
}
