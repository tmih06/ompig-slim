package codingagent

import (
	"strconv"
	"testing"

	"github.com/MichaelKinsy/PiG/tui"
)

func TestTranscriptHistoryPreservesReadingAnchorAfterNotices(t *testing.T) {
	for _, top := range []int{0, 6} {
		t.Run(strconv.Itoa(top), func(t *testing.T) {
			path, _ := pagedFixture(t, 30)
			mode := resumedCacheMode(loadFixtureSession(t, path))
			mode.headerBox = tui.NewContainer(tui.NewSpacer(2))
			mode.loadedResourcesContainer = tui.NewContainer(tui.NewSpacer(2))
			mode.chatContainer.Add(tui.NewSpacer(2))
			mode.renderSessionEntriesWithHistory(true)
			document := tui.NewContainer(mode.headerBox, mode.loadedResourcesContainer, mode.chatContainer)
			view := tui.NewScrollView(document, tui.ScrollViewOptions{})
			defer view.Dispose()
			mode.transcriptScrollView = view
			before := view.Render(100)
			view.UpdateLayout(len(before), 3, nil)
			view.ScrollTo(top)
			beforeTop := view.ScrollTop()
			mode.pageTranscriptHistory()
			after := view.Render(100)
			view.UpdateLayout(len(after), 3, nil)
			afterTop := view.ScrollTop()
			if top == 0 && afterTop != 0 {
				t.Fatalf("history below the visible header moved the viewport to %d", afterTop)
			}
			if top == 6 && afterTop <= beforeTop {
				t.Fatalf("history above the reading anchor did not shift the viewport: %d -> %d", beforeTop, afterTop)
			}
			for row := 0; row < 3; row++ {
				if before[beforeTop+row] != after[afterTop+row] {
					t.Fatalf("visible row %d changed after paging: %q -> %q", row, before[beforeTop+row], after[afterTop+row])
				}
			}
		})
	}
}

func TestTranscriptHistoryPreservesLiveWarmingNotice(t *testing.T) {
	path, _ := pagedFixture(t, 5)
	mode := resumedCacheMode(loadFixtureSession(t, path))
	mode.renderSessionEntriesWithHistory(true)
	notice := &cacheWarmingUsageNotice{}
	mode.cacheWarmingUsage = notice
	mode.pageTranscriptHistory()
	if mode.cacheWarmingUsage != notice {
		t.Fatal("paging replaced the live cache-warming notice with historical state")
	}
}
