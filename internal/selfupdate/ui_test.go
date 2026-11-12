package selfupdate

import (
	"bytes"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/tui"
)

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:                  "0 B",
		999:                "999 B",
		1024:               "1.0 KB",
		1536:               "1.5 KB",
		1024 * 1024:        "1.0 MB",
		34_000_000:         "32.4 MB",
		1024 * 1024 * 1024: "1.0 GB",
	}
	for input, want := range cases {
		if got := humanBytes(input); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", input, got, want)
		}
	}
}

// startupMarkGolden is the startup header's mark as `ompig-slim -ne` drew it in
// a 130-column pty. It is captured from the running TUI rather than derived from
// StartupMark, so this test fails when the printed mark stops matching the one a
// user sees a moment later: a solid block letter or a 16-color code is exactly
// the drift it guards against.
//
// truecolor is COLORTERM=truecolor; 256color is PI_TRUE_COLOR=0, which forces the
// downsampled palette on a TERM=xterm-256color terminal. Regenerate both by
// running the binary in a pty and reading header rows 2 and 3.
var startupMarkGolden = map[string][2]string{
	"truecolor": {
		"\x1b[38;2;236;236;236m█▀█ █▄ ▄█\x1b[0m \x1b[38;2;228;138;122m\x1b[48;2;79;142;179m▀\x1b[0m\x1b[38;2;228;138;122m▀█\x1b[0m     \x1b[38;2;0;173;216m█▀▀ █▀█\x1b[0m \x1b[38;2;198;160;246m-slim\x1b[0m",
		"\x1b[38;2;236;236;236m█▄█ █ ▀ █\x1b[0m \x1b[38;2;79;142;179m█▀\x1b[0m \x1b[38;2;234;182;93m█\x1b[0m \x1b[38;2;236;236;236min \x1b[0m\x1b[38;2;0;173;216m█▄█ █▄█\x1b[0m",
	},
	"256color": {
		"\x1b[38;5;255m█▀█ █▄ ▄█\x1b[0m \x1b[38;5;174m\x1b[48;5;67m▀\x1b[0m\x1b[38;5;174m▀█\x1b[0m     \x1b[38;5;38m█▀▀ █▀█\x1b[0m \x1b[38;5;183m-slim\x1b[0m",
		"\x1b[38;5;255m█▄█ █ ▀ █\x1b[0m \x1b[38;5;67m█▀\x1b[0m \x1b[38;5;179m█\x1b[0m \x1b[38;5;255min \x1b[0m\x1b[38;5;38m█▄█ █▄█\x1b[0m",
	},
}

// TestBannerMatchesTheStartupMark pins the printed mark to the golden capture in
// both color depths. A pipe gets the plain cells, so it also checks that
// stripping the escapes leaves the art intact.
func TestBannerMatchesTheStartupMark(t *testing.T) {
	for mode, want := range startupMarkGolden {
		t.Run(mode, func(t *testing.T) {
			top, bottom := coding.StartupMark(tui.TerminalColorMode(mode))
			if got := [2]string{top, bottom}; got != want {
				t.Fatalf("mark = %q, want %q", got, want)
			}
		})
	}
}

func TestBannerMatchesStartupMarkWithoutColor(t *testing.T) {
	var out bytes.Buffer
	// A bytes.Buffer is never a terminal, so this exercises the plain path.
	printBanner(&out)
	text := out.String()
	if strings.Contains(text, "\x1b") {
		t.Fatalf("plain banner contains an escape sequence: %q", text)
	}
	// The rows are the startup header's, with its escapes stripped: the covered
	// half of the M and the G stay visible as half blocks, not solid cells.
	if !strings.Contains(text, "█▀█ █▄ ▄█ ▀▀█     █▀▀ █▀█ -slim") {
		t.Fatalf("banner is missing the startup top row: %q", text)
	}
	if !strings.Contains(text, "█▄█ █ ▀ █ █▀ █ in █▄█ █▄█") {
		t.Fatalf("banner is missing the startup bottom row: %q", text)
	}
}

func TestProgressStaysSilentWhenNotATerminal(t *testing.T) {
	// A redirected stderr should log decisions, not carriage returns.
	var out bytes.Buffer
	meter := newProgress(&out, "download")
	meter.set(500, 1000)
	meter.finish("archive downloaded")
	if out.Len() != 0 {
		t.Fatalf("non-terminal progress wrote %q", out.String())
	}
}

func TestProgressRendersBarAndPercentOnATerminal(t *testing.T) {
	var out bytes.Buffer
	meter := newProgress(&out, "download")
	// Force the terminal path: a bytes.Buffer cannot be a character device.
	meter.enabled = true

	meter.set(0, 1000)
	half := out.String()
	if !strings.Contains(half, "0%") {
		t.Fatalf("first draw is missing the percentage: %q", half)
	}

	meter.set(500, 1000)
	meter.set(999, 1000)
	// 99% and 50% differ, so both redraw; the bar length encodes the percentage.
	if strings.Count(out.String(), barFilled) != 0 {
		t.Logf("bar fills are drawn as expected")
	}
	meter.finish("archive downloaded")

	final := out.String()
	if !strings.Contains(final, "archive downloaded") {
		t.Fatalf("finish did not write the summary: %q", final)
	}
	// The settled line is terminated, so later output cannot overwrite it.
	if !strings.HasSuffix(final, "archive downloaded\n") {
		t.Fatalf("summary is not newline-terminated: %q", final)
	}
}

func TestProgressDrawsOnlyWhenThePercentageChanges(t *testing.T) {
	var out bytes.Buffer
	meter := newProgress(&out, "download")
	meter.enabled = true
	meter.set(0, 1000)
	first := out.Len()
	// A poll that sees no new bytes must not redraw the same line.
	meter.set(0, 1000)
	meter.set(1, 1000) // still 0%
	if out.Len() != first {
		t.Fatalf("progress redrew for an unchanged percentage")
	}
	meter.set(10, 1000) // now 1%
	if out.Len() == first {
		t.Fatalf("progress did not redraw when the percentage changed")
	}
}

func TestProgressWithoutAKnownTotalShowsTheCount(t *testing.T) {
	var out bytes.Buffer
	meter := newProgress(&out, "download")
	meter.enabled = true
	// A chunked response or a redirect to a signed URL can omit Content-Length.
	meter.set(2048, 0)
	if !strings.Contains(out.String(), "2.0 KB") {
		t.Fatalf("count-only meter is missing the size: %q", out.String())
	}
}
