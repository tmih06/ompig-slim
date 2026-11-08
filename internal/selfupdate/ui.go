package selfupdate

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// The visual language shared with scripts/install.sh. Both surfaces print the
// same logo and the same progress meter so an install and an update read as one
// product. Keep the two files in step: the block art, the bar glyphs, the bar
// width, and the tagline are duplicated by design, because the installer must
// run with no Go toolchain and the updater must run with no shell.
const (
	// barWidth is the number of cells in a full progress bar. It fits an
	// 80-column terminal beside the label and the percentage.
	barWidth = 32

	// barFilled and barEmpty draw the meter. They are single-width in the
	// terminals this project supports, so the bar never shifts as it fills.
	barFilled = "━"
	barEmpty  = "─"
)

// brandEscape is the ANSI SGR sequence for one logo color. The values are the
// fills used by assets/ompig-banner.svg: white for "OM", coral for the top of
// the P, blue for its stem, yellow for the "i", purple for the tagline.
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiWhite  = "\x1b[97m"
	ansiCoral  = "\x1b[91m"
	ansiBlue   = "\x1b[94m"
	ansiYellow = "\x1b[93m"
	ansiPurple = "\x1b[95m"
	ansiCyan   = "\x1b[96m"
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

// logoLines is the startup mark: the product initials in block glyphs, then the
// tagline. Each color run is a slice so a plain render can drop the escapes and
// keep the exact same geometry.
type logoRun struct {
	text  string
	color string
}

// printBanner writes the ompig-slim mark and tagline.
//
// On a terminal the initials carry the brand colors from the SVG banner; in a
// pipe or a log the same text is printed without escapes, so the art still
// lines up. Callers print it once per run, before progress.
func printBanner(w io.Writer) {
	color := isTerminal(w)
	paint := func(run logoRun) string {
		if !color || run.color == "" {
			return run.text
		}
		return run.color + run.text + ansiReset
	}
	// Five block rows spelling "OMP" with the Pi mark's coral P and blue stem,
	// followed by the yellow "i" that the banner SVG adds between GO's rows.
	rows := [][]logoRun{
		{{" ██████╗ ███╗   ███╗", ansiWhite}, {"██████╗ ", ansiCoral}, {"██╗", ansiYellow}},
		{{"██╔═══██╗████╗ ████║", ansiWhite}, {"██╔══██╗", ansiCoral}, {"██║", ansiYellow}},
		{{"██║   ██║██╔████╔██║", ansiWhite}, {"██████╔╝", ansiCoral}, {"██║", ansiYellow}},
		{{"╚██████╔╝██║ ╚═╝ ██║", ansiWhite}, {"██║     ", ansiBlue}, {"██║", ansiYellow}},
		{{" ╚═════╝ ╚═╝     ╚═╝", ansiWhite}, {"╚═╝     ", ansiBlue}, {"╚═╝", ansiYellow}},
	}
	var out strings.Builder
	out.WriteString("\n")
	for _, row := range rows {
		for _, run := range row {
			out.WriteString(paint(run))
		}
		out.WriteString("\n")
	}
	out.WriteString("\n")
	tag := "  in Go · slim harness"
	if color {
		tag = ansiPurple + tag + ansiReset
	}
	out.WriteString(tag)
	out.WriteString("\n\n")
	_, _ = io.WriteString(w, out.String())
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
		return fmt.Sprintf("%.1f KB", float64(n)/(1024))
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	}
}
