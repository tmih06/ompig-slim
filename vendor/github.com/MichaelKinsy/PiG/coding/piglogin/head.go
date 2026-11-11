package piglogin

import (
	"image/color"
	"maps"
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/tui"
)

// The sprite's mascot head is its 16-by-14 artwork (MascotSpriteFor), the same pixels /sprite preview draws: two pixel
// rows per line in half blocks, so HeadCells cells by HeadRows lines.
const (
	HeadWidth  = extension.LoginMascotWidth
	HeadHeight = extension.LoginMascotHeight
	HeadCells  = HeadWidth
	HeadRows   = HeadHeight / 2
)

// HeadFor returns the variant's pig head: its 16-by-14 pig.
func HeadFor(variant Variant) []string { return MascotSpriteFor(variant) }

// HeadPixel is one opaque pixel of a pig head: its column and row in the HeadWidth by HeadHeight grid, its symbol and its
// color.
type HeadPixel struct {
	X, Y   int
	Symbol byte
	Color  color.RGBA
}

// HeadPixels returns the opaque pixels of the variant's pig head in row-major order, the pixels HeadLines draws.
func HeadPixels(variant Variant) []HeadPixel {
	palette := paletteFor(variant)
	head := HeadFor(variant)
	pixels := make([]HeadPixel, 0, HeadWidth*HeadHeight)
	for y, row := range head {
		for x := range len(row) {
			if value, ok := palette[row[x]]; ok && row[x] != '.' && value.A != 0 {
				pixels = append(pixels, HeadPixel{X: x, Y: y, Symbol: row[x], Color: value})
			}
		}
	}
	return pixels
}

// SameHead reports whether two variants draw the same pig head: the same grid in the same colors.
// pig additive (D91): a frontend's mark is the active sprite's head, rebuilt only when the head changes.
func SameHead(a, b Variant) bool {
	return slices.Equal(HeadFor(a), HeadFor(b)) && a.Body == b.Body && a.Highlight == b.Highlight && a.Snout == b.Snout &&
		a.Blush == b.Blush && a.InnerEar == b.InnerEar && maps.Equal(a.PaletteOverrides, b.PaletteOverrides)
}

// HeadLines draws the variant's pig head in half blocks, HeadCells cells wide on each of HeadRows lines: the upper half
// block in the top pixel's color over the bottom pixel's, the lower half block where only the bottom pixel is opaque.
// Every cell ends in a reset, so a transparent pixel shows the terminal's background.
func HeadLines(variant Variant, mode tui.TerminalColorMode) []string {
	grid := make([][]*color.RGBA, HeadHeight)
	for y := range grid {
		grid[y] = make([]*color.RGBA, HeadWidth)
	}
	for _, pixel := range HeadPixels(variant) {
		value := pixel.Color
		grid[pixel.Y][pixel.X] = &value
	}
	return halfBlockLines(grid, mode)
}

// halfBlockLines draws a pixel grid of even height in half blocks, two pixel rows per line; a nil pixel is transparent.
func halfBlockLines(grid [][]*color.RGBA, mode tui.TerminalColorMode) []string {
	lines := make([]string, len(grid)/2)
	for line := range lines {
		var b strings.Builder
		for x := range grid[line*2] {
			top, bottom := grid[line*2][x], grid[line*2+1][x]
			switch {
			case top != nil && bottom != nil:
				b.WriteString(tui.ForegroundAnsi(markColor(*top), mode) + tui.BackgroundAnsi(markColor(*bottom), mode) + "▀" + markReset)
			case top != nil:
				b.WriteString(tui.ForegroundAnsi(markColor(*top), mode) + "▀" + markReset)
			case bottom != nil:
				b.WriteString(tui.ForegroundAnsi(markColor(*bottom), mode) + "▄" + markReset)
			default:
				b.WriteByte(' ')
			}
		}
		lines[line] = b.String()
	}
	return lines
}

// ArtWidth and ArtRows are the cells of a sprite's full art: its 16-pixel mascot, 14 pixels tall, at two pixel rows per
// line.
const (
	ArtWidth = extension.LoginMascotWidth
	ArtRows  = extension.LoginMascotHeight / 2
)

// ArtLines draws the variant's full art in half blocks: its 16-by-14 mascot, colored by the sprite's palette.
// pig divergence (D2): the art is the mascot alone. The upstream artwork pairs it with a 32-pixel "PiG." hero wordmark,
// which is a different product's name; this build's product mark is the OM + Pi startup logo (startup_logo.go), so
// /sprite preview shows the sprite itself and lets its name and tagline name it.
func ArtLines(variant Variant, mode tui.TerminalColorMode) []string {
	grid := make([][]*color.RGBA, extension.LoginMascotHeight)
	palette := paletteFor(variant)
	art := MascotSpriteFor(variant)
	for y := range grid {
		grid[y] = make([]*color.RGBA, extension.LoginMascotWidth)
		for x := range len(art[y]) {
			if value, ok := palette[art[y][x]]; ok && art[y][x] != '.' && value.A != 0 {
				cell := value
				grid[y][x] = &cell
			}
		}
	}
	return halfBlockLines(grid, mode)
}
