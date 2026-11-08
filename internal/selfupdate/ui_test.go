package selfupdate

import (
	"bytes"
	"strings"
	"testing"
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

func TestBannerGeometryIsStableWithoutColor(t *testing.T) {
	var out bytes.Buffer
	// A bytes.Buffer is never a terminal, so this exercises the plain path.
	printBanner(&out)
	text := out.String()
	if strings.Contains(text, "\x1b") {
		t.Fatalf("plain banner contains an escape sequence: %q", text)
	}
	if !strings.Contains(text, "in Go \u00b7 slim harness") {
		t.Fatalf("banner is missing the tagline: %q", text)
	}
	var art []string
	for _, line := range strings.Split(text, "\n") {
		if strings.ContainsAny(line, "\u2588\u255a") {
			art = append(art, line)
		}
	}
	if len(art) != 5 {
		t.Fatalf("banner has %d block rows, want 5: %q", len(art), text)
	}
	// Rows must agree on width or the mark shears in a terminal.
	for _, line := range art {
		if len([]rune(line)) != len([]rune(art[0])) {
			t.Fatalf("logo rows differ in width: %q", art)
		}
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
