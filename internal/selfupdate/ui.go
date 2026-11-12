package selfupdate

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// The progress meter is this package's own rendering: the installer and the
// updater share it so an install and an update read as one product, and
// scripts/install.sh duplicates it because the installer must run with no Go
// toolchain. The mark above it is not duplicated — coding.StartupMark owns it.
const (
	// barWidth is the number of cells in a full progress bar. It fits an
	// 80-column terminal beside the label and the percentage.
	barWidth = 32

	// barFilled and barEmpty draw the meter. They are single-width in the
	// terminals this project supports, so the bar never shifts as it fills.
	barFilled = "━"
	barEmpty  = "─"
)

// The SGR codes used by the progress meter. The mark itself carries its own brand colors, which come from
// coding.StartupMark so an update cannot drift from the startup header.
const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
	ansiCyan  = "\x1b[96m"
)

// isTerminal reports whether w is a character device, so the caller can choose
// colored, in-place output over plain lines that survive a pipe or a log file.
// A writer that is not an *os.File (a test buffer, an http response) is never a
// terminal.
func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// printBanner writes the startup TUI's two-line mark, then a blank line.
//
// The art is not restated here: coding.StartupMark renders the same rows the startup header draws, in the color depth
// the terminal supports, so a change to the mark reaches every surface at once. In a pipe or a log the escapes are
// stripped instead of omitted, which keeps the geometry because every run is a plain span of printable cells.
func printBanner(w io.Writer) {
	top, bottom := coding.StartupMark(codingMarkSurface(w))
	if !isTerminal(w) {
		top, bottom = widthx.StripAnsi(top), widthx.StripAnsi(bottom)
	}
	_, _ = io.WriteString(w, "\n"+top+"\n"+bottom+"\n\n")
}

// codingMarkSurface is the color depth the mark renders in: the one the startup header would pick, decided by terminal
// capabilities (COLORTERM, TERM_PROGRAM, an explicit PI_TRUE_COLOR). A pipe gets 256-color because nothing will read
// the escapes anyway; the plain form prints the same cells.
func codingMarkSurface(w io.Writer) tui.TerminalColorMode {
	if !isTerminal(w) {
		return tui.TerminalColorMode256
	}
	return tui.GetTerminalColorMode()
}

// progress renders a single-line meter that updates in place on a terminal and
// stays silent where it cannot: a redirected stderr should log decisions, not
// forty carriage returns.
type progress struct {
	w       io.Writer
	enabled bool
	label   string
	// lastPercent records the drawn state so a poll that sees no new bytes does
	// not redraw the same line.
	lastPercent int
	// drew tracks whether a meter line is currently open and needs its
	// terminating newline before other output.
	drew bool
}

// newProgress returns a meter for label. It is live only when w is a terminal;
// otherwise its methods are no-ops and the caller's summary lines carry the
// information.
func newProgress(w io.Writer, label string) *progress {
	return &progress{w: w, enabled: isTerminal(w), label: label, lastPercent: -1}
}

// set reports byte progress against a known total. total <= 0 means the size is
// unknown, in which case the meter shows the running count without a bar.
func (p *progress) set(done, total int64) {
	if !p.enabled {
		return
	}
	if total > 0 {
		percent := int(done * 100 / total)
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
		if percent == p.lastPercent {
			return
		}
		p.lastPercent = percent
		filled := percent * barWidth / 100
		bar := strings.Repeat(barFilled, filled) + strings.Repeat(barEmpty, barWidth-filled)
		// The label and the bar draw bright; the counts stay dim so the moving
		// part of the line is the meter itself. Two trailing spaces erase a
		// glyph left by a previously longer count.
		fmt.Fprintf(p.w, "\r  %s%s%s %s%s%s %3d%% %s%s / %s%s  ",
			ansiBold, p.label, ansiReset,
			ansiCyan, bar, ansiReset,
			percent,
			ansiDim, humanBytes(done), humanBytes(total), ansiReset)
	} else {
		fmt.Fprintf(p.w, "\r  %s %s    %s", p.label, ansiDim, humanBytes(done))
	}
	p.drew = true
}

// finish closes an open meter line and turns it into a permanent result. It
// prints nothing when no meter was drawn, so non-terminal output keeps exactly
// the summary lines the caller writes itself.
func (p *progress) finish(summary string) {
	if !p.enabled {
		return
	}
	if p.drew {
		// Erase the open line before writing the settled one, so a shorter
		// summary cannot leave bar remnants behind it.
		fmt.Fprint(p.w, "\r\x1b[K")
		p.drew = false
	}
	fmt.Fprintf(p.w, "  %s\n", summary)
}

// humanBytes formats a byte count the way the installer does: one decimal for
// megabytes and gigabytes, whole bytes below a kilobyte.
func humanBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	}
}
