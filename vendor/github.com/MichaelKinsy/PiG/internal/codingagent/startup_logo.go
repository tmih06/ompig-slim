// Ports packages/coding-agent/src/modes/interactive/components/pi-logo.ts of omp-slim.
package codingagent

import (
	"strings"

	"github.com/MichaelKinsy/PiG/tui"
)

// The logo's fixed brand colors, independent of the theme (pi-logo.ts WHITE, CORAL, BLUE, YELLOW,
// LIGHT_PURPLE). They follow the terminal's color mode.
var (
	logoWhite       = logoBrandColor(236, 236, 236)
	logoCoral       = logoBrandColor(228, 138, 122)
	logoBlue        = logoBrandColor(79, 142, 179)
	logoYellow      = logoBrandColor(234, 182, 93)
	logoLightPurple = logoBrandColor(198, 160, 246)
)

const logoReset = "\x1b[0m"

// piLogoCells and piLogoRows are Pi's original four-cell logo: 4 cells wide and 2 lines tall.
const (
	piLogoCells = 4
	piLogoRows  = 2
)

// omPrefixCells is the white "OM" prefix: O is 3 cells, M is 5, one space between them.
const omPrefixCells = 3 + 1 + 5

// logoTag is the plain-text suffix following the block-letter GO mark.
const logoTag = "-slim"

// logoGlyphs are the fork's prefix letters, four pixel rows tall each. Line one draws pixel rows 0-1 and
// line two draws rows 2-3. M is five columns wide: at this height a three-column M is indistinguishable
// from H.
var logoGlyphs = map[byte][4]string{
	'O': {"███", "█ █", "█ █", "███"},
	'M': {"█   █", "██ ██", "█ █ █", "█   █"},
	'G': {"███", "█  ", "█ █", "███"},
}

func logoBrandColor(r, g, b float64) tui.Color {
	color, err := tui.NewRgbColor(r, g, b)
	if err != nil {
		panic(err)
	}
	return color
}

// renderOmLine renders "OM" in white at topRow (0 or 2), each column collapsing two pixel rows into one
// block. Characters are separated by exactly one space, with no trailing space, so the caller owns the
// gap between the prefix and Pi's mark.
func renderOmLine(topRow int, mode tui.TerminalColorMode) string {
	var b strings.Builder
	b.WriteString(tui.ForegroundAnsi(logoWhite, mode))
	for index, character := range [2]byte{'O', 'M'} {
		if index > 0 {
			b.WriteByte(' ')
		}
		glyph := logoGlyphs[character]
		top, bottom := []rune(glyph[topRow]), []rune(glyph[topRow+1])
		for column := range top {
			filledTop := top[column] == '█'
			filledBottom := bottom[column] == '█'
			switch {
			case filledTop && filledBottom:
				b.WriteRune('█')
			case filledTop:
				b.WriteRune('▀')
			case filledBottom:
				b.WriteRune('▄')
			default:
				b.WriteByte(' ')
			}
		}
	}
	b.WriteString(logoReset)
	return b.String()
}

// piLogoLines is the startup mark: "OM" in white half blocks followed by Pi's original four-cell logo,
// unchanged. The prefix identifies the fork while Pi's mark keeps coral, blue and yellow.
func piLogoLines(mode tui.TerminalColorMode) (string, string) {
	fg := func(color tui.Color) string { return tui.ForegroundAnsi(color, mode) }
	// The fourth cell of the top line is empty, so it is padded to the same width as the bottom line.
	piTop := fg(logoCoral) + tui.BackgroundAnsi(logoBlue, mode) + "▀" + logoReset + fg(logoCoral) + "▀█" + logoReset + " "
	piBottom := fg(logoBlue) + "█▀" + logoReset + " " + fg(logoYellow) + "█" + logoReset
	return renderOmLine(0, mode) + " " + piTop, renderOmLine(2, mode) + " " + piBottom
}

// supportsPiLogo reports whether the terminal renders the half-block logo correctly. Apple Terminal draws
// gaps between rows and misaligns the half blocks, so it gets the text wordmark instead. A variable so
// tests can select either.
var supportsPiLogo = func() bool { return !tui.IsAppleTerminalSession() }

// piWordmark is the text fallback for the logo: the white "OM" prefix, then Pi's coral "P" and yellow "i".
func piWordmark(mode tui.TerminalColorMode) string {
	return tui.ForegroundAnsi(logoWhite, mode) + "OM" + logoReset +
		tui.ForegroundAnsi(logoCoral, mode) + "P" + logoReset +
		tui.ForegroundAnsi(logoYellow, mode) + "i" + logoReset
}

// productTag is the product qualifier shown after the mark. Light purple at full intensity: it is brand
// text like the logo, so it deliberately avoids the theme's dim tokens.
func productTag(mode tui.TerminalColorMode) string {
	return tui.ForegroundAnsi(logoLightPurple, mode) + logoTag + logoReset
}

// renderGoLine draws GO as four-row block glyphs, collapsing pairs of pixel rows into terminal blocks.
func renderGoLine(topRow int, mode tui.TerminalColorMode) string {
	var b strings.Builder
	b.WriteString(tui.ForegroundAnsi(logoBrandColor(0, 173, 216), mode))
	for index, character := range []byte{'G', 'O'} {
		if index > 0 {
			b.WriteByte(' ')
		}
		glyph := logoGlyphs[character]
		top, bottom := []rune(glyph[topRow]), []rune(glyph[topRow+1])
		for column := range top {
			switch {
			case top[column] == '█' && bottom[column] == '█':
				b.WriteRune('█')
			case top[column] == '█':
				b.WriteRune('▀')
			case bottom[column] == '█':
				b.WriteRune('▄')
			default:
				b.WriteByte(' ')
			}
		}
	}
	b.WriteString(logoReset)
	return b.String()
}
