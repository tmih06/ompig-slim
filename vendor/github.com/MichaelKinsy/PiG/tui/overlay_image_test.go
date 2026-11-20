package tui

import (
	"bytes"
	"strings"
	"testing"
)

type overlayImageFixture struct{ lines []string }

func (c overlayImageFixture) Render(int) []string { return c.lines }
func (overlayImageFixture) Invalidate()           {}

func TestOverlayHidesKittyImagesThatCoverItsCells(t *testing.T) {
	image := EncodeKitty("aGVsbG8=", 10, 3, 0)
	snapshot := overlayStateSnapshot{
		MountedAny: true,
		Entries: []overlayEntry{{
			component:        overlayImageFixture{lines: []string{"overlay"}},
			mounted:          true,
			evaluatedVisible: true,
			opts: OverlayOptions{
				width: overlayCells(7),
				row:   overlayCells(1),
				col:   overlayCells(0),
			},
		}},
	}

	lines, _ := composeOverlaySnapshotWithStats([]string{image, "", ""}, snapshot, 20, 3, false)
	if strings.Contains(lines[0], "\x1b_G") {
		t.Fatalf("image placement still covers the overlay: %q", lines[0])
	}
	if !strings.Contains(lines[1], "overlay") {
		t.Fatalf("overlay was not composited over the image's covered row: %q", lines[1])
	}
}

func TestOverlayKeepsKittyImagesOutsideItsCells(t *testing.T) {
	image := EncodeKitty("aGVsbG8=", 10, 3, 0)
	snapshot := overlayStateSnapshot{
		MountedAny: true,
		Entries: []overlayEntry{{
			component:        overlayImageFixture{lines: []string{"overlay"}},
			mounted:          true,
			evaluatedVisible: true,
			opts: OverlayOptions{
				width: overlayCells(5),
				row:   overlayCells(1),
				col:   overlayCells(10),
			},
		}},
	}

	lines, _ := composeOverlaySnapshotWithStats([]string{image, "", ""}, snapshot, 20, 3, false)
	if !strings.Contains(lines[0], "\x1b_G") {
		t.Fatal("image placement outside the overlay was hidden")
	}
}

func TestOverlayHidesITermImagesThatCoverItsCells(t *testing.T) {
	image := "\x1b[2A" + EncodeITerm2("aGVsbG8=", ITerm2Options{Width: 10, Height: "auto"})
	snapshot := overlayStateSnapshot{
		MountedAny: true,
		Entries: []overlayEntry{{
			component:        overlayImageFixture{lines: []string{"overlay"}},
			mounted:          true,
			evaluatedVisible: true,
			opts: OverlayOptions{
				width: overlayCells(7),
				row:   overlayCells(1),
				col:   overlayCells(0),
			},
		}},
	}

	lines, _ := composeOverlaySnapshotWithStats([]string{"", "", image}, snapshot, 20, 3, false)
	if strings.Contains(lines[2], "1337;File=") || strings.Contains(lines[2], "\x1b[2A") {
		t.Fatalf("iTerm image placement still covers the overlay: %q", lines[2])
	}
}

func TestAltScreenOverlayClearsPreviouslyPlacedKittyImage(t *testing.T) {
	imageID := AllocateImageID()
	RegisterKittyImageMetadata(KittyImageMetadata{
		ImageID: imageID, Columns: 10, Rows: 2, WidthPx: 90, HeightPx: 36,
	})
	image := EncodeKitty("aGVsbG8=", 10, 2, imageID)
	var output bytes.Buffer
	renderer := newTuiAltScreen(&output, false, TuiAltScreenOptions{})
	renderer.fixedSize = true
	renderer.setSize(20, 4)
	renderer.imageProtocol = ImageProtocolKitty
	renderer.altScreenActive = true
	renderer.layoutRoot = &overlayImageFixture{lines: []string{image, "", "", ""}}
	renderer.doRender()
	if !strings.Contains(output.String(), image) {
		t.Fatal("initial frame did not place the Kitty image")
	}

	output.Reset()
	renderer.applyOverlayCommand(overlayCommand{
		kind: overlayMount, component: &overlayImageFixture{lines: []string{"overlay"}}, visible: true,
		options: OverlayOptions{width: overlayCells(7), row: overlayCells(1), col: overlayCells(0)},
	})
	renderer.doRender()

	if !strings.Contains(output.String(), DeleteAllKittyPlacements()) {
		t.Fatal("overlay frame did not clear the previously placed Kitty image")
	}
	for _, line := range renderer.GetScreenLines() {
		if strings.Contains(line, "\x1b_G") {
			t.Fatalf("overlay frame retained a Kitty placement: %q", line)
		}
	}
	if screen := strings.Join(renderer.GetScreenLines(), "\n"); !strings.Contains(screen, "overlay") {
		t.Fatalf("overlay was not drawn after clearing the Kitty image: %q", screen)
	}
}
