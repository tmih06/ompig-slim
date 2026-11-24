package tui

import (
	"fmt"
	"sync"
	"time"
)

// Ports pi-tui's scroll-view.ts: a single-child vertical scroll viewport with an
// optional auto-hiding transient scrollbar. Names and structure mirror upstream
// 1:1. Forced Go mechanics: the scrollbar-hide setTimeout becomes an owned
// time.AfterFunc guarded by a mutex plus a generation counter, because
// time.AfterFunc fires on a separate goroutine and Stop() races a fired-but-
// pending callback (JS setTimeout runs on the single event loop, so upstream
// needs neither); the four upstream throws become panics with identical
// messages; and the "hidden"|"auto"|"always" scrollbar mode stays a string.

// ScrollViewOptions configure a ScrollView. Empty string fields take upstream's
// default; ScrollbarHideDelayMs nil defaults to 1000ms.
type ScrollViewOptions struct {
	Axis                 string // "" (unset) or "vertical"
	Follow               string // "none" | "end"
	Primary              bool
	Overscroll           string // "chain" | "contain"
	Scrollbar            ScrollViewScrollbar
	ScrollbarTrackStyle  func(text string) string
	ScrollbarThumbStyle  func(text string) string
	ScrollbarHideDelayMs *int
}

// ScrollViewScrollToOptions mirrors upstream ScrollViewScrollToOptions.
type ScrollViewScrollToOptions struct {
	// DisableFollow keeps follow-end disabled even when the target is the
	// current content end.
	DisableFollow bool
}

// ScrollViewScrollbar is the scrollbar mode: "hidden", "auto" or "always" (components/scroll-view.ts ScrollViewScrollbar).
type ScrollViewScrollbar string

const (
	ScrollbarHidden ScrollViewScrollbar = "hidden"
	ScrollbarAuto   ScrollViewScrollbar = "auto"
	ScrollbarAlways ScrollViewScrollbar = "always"
)

// ScrollView ports pi-tui's ScrollView. It embeds pig's Container (holding the
// single child) and owns the transient-scrollbar timer.
type ScrollView struct {
	*Container
	child                Component
	followEnd            bool
	primary              bool
	overscroll           string
	scrollbarTrackStyle  func(text string) string
	scrollbarThumbStyle  func(text string) string
	scrollbarHideDelayMs int

	mu                        sync.Mutex
	currentScrollbar          ScrollViewScrollbar
	currentScrollTop          int
	contentHeight             int
	currentViewportHeight     int
	followingEnd              bool
	followSuppressedAtEnd     bool
	requestRenderCallback     func()
	onScrollChanged           func(delta int)
	renderContentWidth        int
	transientScrollbarVisible bool
	scrollbarActive           bool
	scrollbarHideTimer        *time.Timer
	scrollbarHideGen          int // invalidates a fired-but-pending hide callback
}

// NewScrollView constructs a ScrollView. It panics on an unsupported axis,
// mirroring upstream's constructor throw.
func NewScrollView(component Component, options ScrollViewOptions) *ScrollView {
	if options.Axis != "" && options.Axis != "vertical" {
		panic(fmt.Sprintf("Unsupported ScrollView axis: %s", options.Axis))
	}
	follow := options.Follow
	if follow == "" {
		follow = "none"
	}
	overscroll := options.Overscroll
	if overscroll == "" {
		overscroll = "chain"
	}
	scrollbar := options.Scrollbar
	if scrollbar == "" {
		scrollbar = "hidden"
	}
	trackStyle := options.ScrollbarTrackStyle
	if trackStyle == nil {
		trackStyle = func(text string) string { return "\x1b[90m" + text + "\x1b[39m" }
	}
	thumbStyle := options.ScrollbarThumbStyle
	if thumbStyle == nil {
		thumbStyle = func(text string) string { return "\x1b[37m" + text + "\x1b[39m" }
	}
	delay := 1000
	if options.ScrollbarHideDelayMs != nil {
		delay = *options.ScrollbarHideDelayMs
	}
	sv := &ScrollView{
		Container:            NewContainer(component),
		child:                component,
		followEnd:            follow == "end",
		primary:              options.Primary,
		overscroll:           overscroll,
		scrollbarTrackStyle:  trackStyle,
		scrollbarThumbStyle:  thumbStyle,
		scrollbarHideDelayMs: max(0, delay),
		currentScrollbar:     scrollbar,
	}
	sv.followingEnd = sv.followEnd
	return sv
}

// ScrollTop reports the current scroll offset.
func (s *ScrollView) ScrollTop() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentScrollTop
}

// Primary reports whether this is the primary scroll view.
func (s *ScrollView) Primary() bool { return s.primary }

// Overscroll reports the overscroll mode.
func (s *ScrollView) Overscroll() string { return s.overscroll }

// ViewportHeight reports the current viewport height.
func (s *ScrollView) ViewportHeight() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentViewportHeight
}

// IsFollowingEnd reports whether the view is pinned to the end.
func (s *ScrollView) IsFollowingEnd() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.followingEnd
}

// Scrollbar reports the current scrollbar mode.
func (s *ScrollView) Scrollbar() ScrollViewScrollbar {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentScrollbar
}

// FollowEnd reports whether the view was configured to follow its end.
func (s *ScrollView) FollowEnd() bool { return s.followEnd }

// ScrollbarTrackStyle styles scrollbar track cells: upstream's readonly `scrollbarTrackStyle` is a function property, so it is called with the text.
// upstream: packages/tui/src/components/scroll-view.ts:27
func (s *ScrollView) ScrollbarTrackStyle(text string) string { return s.scrollbarTrackStyle(text) }

// ScrollbarThumbStyle styles scrollbar thumb cells (upstream scroll-view.ts:28).
func (s *ScrollView) ScrollbarThumbStyle(text string) string { return s.scrollbarThumbStyle(text) }

// IsScrollbarActive reports whether the pointer is hovering or dragging the scrollbar.
func (s *ScrollView) IsScrollbarActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scrollbarActive
}

// IsScrollbarVisible reports whether the scrollbar should paint this frame.
func (s *ScrollView) IsScrollbarVisible() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentScrollbar == ScrollbarAlways {
		return s.currentViewportHeight > 0
	}
	return s.currentScrollbar == ScrollbarAuto && s.contentHeight > s.currentViewportHeight && s.transientScrollbarVisible
}

// SetScrollbar changes the scrollbar mode.
func (s *ScrollView) SetScrollbar(scrollbar ScrollViewScrollbar) {
	s.mu.Lock()
	if scrollbar == s.currentScrollbar {
		s.mu.Unlock()
		return
	}
	s.currentScrollbar = scrollbar
	if scrollbar != "auto" {
		s.hideTransientScrollbar()
	} else if s.scrollbarActive {
		s.markScrollbarActivity()
	}
	cb := s.requestRenderCallback
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// GetContentWidth returns the child render width, reserving a column for an
// always-on scrollbar.
func (s *ScrollView) GetContentWidth(width int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getContentWidthLocked(width)
}

// getContentWidthLocked is GetContentWidth with s.mu held.
func (s *ScrollView) getContentWidthLocked(width int) int {
	if s.currentScrollbar == ScrollbarAlways && width > 1 {
		return width - 1
	}
	return width
}

// RenderedContentWidth returns the content width the child was last rendered
// at, or 0 before the first render. A caller measuring inserted rows must
// render at this width so the row count matches what the viewport lays out.
func (s *ScrollView) RenderedContentWidth() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.renderContentWidth
}

// SetOnScrollChanged installs a callback invoked after every scroll request,
// even one that could not move the view because it already is at the edge.
// delta is the requested scroll direction: negative is toward the start, so a
// repeat wheel-up at the top still reaches a lazy loader. The callback runs
// with the scroll lock released and must not mutate the component tree
// itself; it should post work to the owning UI loop.
func (s *ScrollView) SetOnScrollChanged(cb func(delta int)) {
	s.mu.Lock()
	s.onScrollChanged = cb
	s.mu.Unlock()
}

// fireScrollChanged runs the scroll-change callback installed by
// SetOnScrollChanged with the requested direction, if any.
func (s *ScrollView) fireScrollChanged(delta int) {
	s.mu.Lock()
	cb := s.onScrollChanged
	s.mu.Unlock()
	if cb != nil {
		cb(delta)
	}
}

// CompensatePrepend shifts the scroll offset by rows, keeping the content the
// viewport shows stable after rows worth of content was spliced in above it.
// A view following its end needs no shift: the next layout pass already pins
// it to the end of the taller content. The shift is capped so the offset
// stays inside the range the next layout accepts.
func (s *ScrollView) CompensatePrepend(rows int) {
	if rows <= 0 {
		return
	}
	s.mu.Lock()
	if s.followingEnd {
		s.mu.Unlock()
		return
	}
	maxScrollTop := max(0, s.contentHeight-s.currentViewportHeight)
	s.currentScrollTop = min(s.currentScrollTop+rows, maxScrollTop+rows)
	s.markScrollbarActivity()
	s.mu.Unlock()
}

// markScrollbarActivity shows the transient scrollbar and arms the hide timer.
// The caller must hold s.mu.
func (s *ScrollView) markScrollbarActivity() {
	if s.currentScrollbar != ScrollbarAuto || s.contentHeight <= s.currentViewportHeight {
		return
	}
	s.transientScrollbarVisible = true
	s.stopHideTimer()
	if s.scrollbarActive {
		return
	}
	gen := s.scrollbarHideGen
	s.scrollbarHideTimer = time.AfterFunc(time.Duration(s.scrollbarHideDelayMs)*time.Millisecond, func() {
		s.onHideTimer(gen)
	})
}

// stopHideTimer stops any armed hide timer and invalidates a pending callback.
// The caller must hold s.mu.
func (s *ScrollView) stopHideTimer() {
	if s.scrollbarHideTimer != nil {
		s.scrollbarHideTimer.Stop()
		s.scrollbarHideTimer = nil
	}
	s.scrollbarHideGen++
}

// onHideTimer fires off the timer goroutine; it hides the transient scrollbar
// unless a newer activity has superseded this generation.
func (s *ScrollView) onHideTimer(gen int) {
	s.mu.Lock()
	if gen != s.scrollbarHideGen {
		s.mu.Unlock()
		return
	}
	s.scrollbarHideTimer = nil
	s.transientScrollbarVisible = false
	cb := s.requestRenderCallback
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// hideTransientScrollbar hides the scrollbar and cancels the hide timer.
// The caller must hold s.mu.
func (s *ScrollView) hideTransientScrollbar() {
	s.transientScrollbarVisible = false
	s.stopHideTimer()
}

// SetScrollbarActive marks the scrollbar as actively dragged (keeping it shown).
func (s *ScrollView) SetScrollbarActive(active bool) {
	s.mu.Lock()
	if active == s.scrollbarActive {
		s.mu.Unlock()
		return
	}
	s.scrollbarActive = active
	s.markScrollbarActivity()
	cb := s.requestRenderCallback
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// ScrollTo scrolls to an absolute offset, clamped to the scrollable range. Mirrors upstream scrollTo(scrollTop, options = {}); the optional options are the second parameter.
func (s *ScrollView) ScrollTo(scrollTop int, opts ...ScrollViewScrollToOptions) {
	var options ScrollViewScrollToOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	s.mu.Lock()
	prev := s.currentScrollTop
	maxScrollTop := max(0, s.contentHeight-s.currentViewportHeight)
	next := max(0, min(maxScrollTop, scrollTop))
	nextFollowSuppressedAtEnd := options.DisableFollow && next == maxScrollTop
	nextFollowingEnd := !nextFollowSuppressedAtEnd && s.followEnd && next == maxScrollTop
	if next == s.currentScrollTop && nextFollowingEnd == s.followingEnd &&
		nextFollowSuppressedAtEnd == s.followSuppressedAtEnd {
		s.mu.Unlock()
		s.fireScrollChanged(scrollTop - prev)
		return
	}
	moved := next != s.currentScrollTop
	s.currentScrollTop = next
	s.followingEnd = nextFollowingEnd
	s.followSuppressedAtEnd = nextFollowSuppressedAtEnd
	if moved {
		s.markScrollbarActivity()
	}
	cb := s.requestRenderCallback
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
	s.fireScrollChanged(scrollTop - prev)
}

// ScrollBy scrolls by a relative number of lines and returns the leftover lines
// that could not be applied (for overscroll chaining).
func (s *ScrollView) ScrollBy(lines int) int {
	s.mu.Lock()
	if lines == 0 {
		s.mu.Unlock()
		s.fireScrollChanged(0)
		return 0
	}
	maxScrollTop := max(0, s.contentHeight-s.currentViewportHeight)
	start := s.currentScrollTop
	if s.followingEnd {
		start = maxScrollTop
	}
	next := max(0, min(maxScrollTop, start+lines))
	moved := next - start
	wasFollowingEnd := s.followingEnd
	s.currentScrollTop = next
	s.followingEnd = s.followEnd && next == maxScrollTop
	s.followSuppressedAtEnd = false
	if moved != 0 {
		s.markScrollbarActivity()
	}
	var cb func()
	if moved != 0 || s.followingEnd != wasFollowingEnd {
		cb = s.requestRenderCallback
	}
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
	s.fireScrollChanged(lines)
	return lines - moved
}

// ScrollToStart scrolls to the top.
func (s *ScrollView) ScrollToStart() {
	s.mu.Lock()
	prev := s.currentScrollTop
	changed := s.currentScrollTop != 0 ||
		s.followingEnd != (s.followEnd && s.contentHeight <= s.currentViewportHeight)
	s.currentScrollTop = 0
	s.followingEnd = s.followEnd && s.contentHeight <= s.currentViewportHeight
	s.followSuppressedAtEnd = false
	var cb func()
	if changed {
		s.markScrollbarActivity()
		cb = s.requestRenderCallback
	}
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
	// Top is always an upward request: even at the start the direction is
	// toward earlier content, which is what a lazy loader keys on.
	s.fireScrollChanged(-max(prev, 1))
}

// ScrollToEnd scrolls to the bottom.
func (s *ScrollView) ScrollToEnd() {
	s.mu.Lock()
	prev := s.currentScrollTop
	next := max(0, s.contentHeight-s.currentViewportHeight)
	changed := s.currentScrollTop != next || s.followingEnd != s.followEnd
	s.currentScrollTop = next
	s.followingEnd = s.followEnd
	s.followSuppressedAtEnd = false
	var cb func()
	if changed {
		s.markScrollbarActivity()
		cb = s.requestRenderCallback
	}
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
	s.fireScrollChanged(next - prev)
}

// UpdateLayout is called by the layout engine each pass with the measured
// content and viewport heights; it clamps the scroll offset and stores the
// render callback the hide timer will invoke.
func (s *ScrollView) UpdateLayout(contentHeight, viewportHeight int, requestRender func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contentHeight = max(0, contentHeight)
	s.currentViewportHeight = max(0, viewportHeight)
	s.requestRenderCallback = requestRender
	maxScrollTop := max(0, s.contentHeight-s.currentViewportHeight)
	if s.followingEnd {
		s.currentScrollTop = maxScrollTop
	} else {
		s.currentScrollTop = max(0, min(s.currentScrollTop, maxScrollTop))
	}
	if s.currentScrollTop < maxScrollTop {
		s.followSuppressedAtEnd = false
	}
	if s.followEnd && s.currentScrollTop == maxScrollTop && !s.followSuppressedAtEnd {
		s.followingEnd = true
	}
	if s.contentHeight <= s.currentViewportHeight {
		s.hideTransientScrollbar()
	}
}

// Add panics: a ScrollView has exactly one child (upstream throw).
func (s *ScrollView) Add(Component) { panic("ScrollView has exactly one child") }

// Remove panics: a ScrollView's child cannot be removed (upstream throw).
func (s *ScrollView) Remove(Component) { panic("ScrollView child cannot be removed") }

// Clear panics: a ScrollView's child cannot be cleared (upstream throw).
func (s *ScrollView) Clear() { panic("ScrollView child cannot be cleared") }

// Render ports upstream ScrollView.render: render the child at the content width,
// padding each line by a column when a scrollbar column is reserved.
func (s *ScrollView) Render(width int) []string {
	s.mu.Lock()
	contentWidth := s.getContentWidthLocked(width)
	s.renderContentWidth = contentWidth
	s.mu.Unlock()
	lines := s.child.Render(contentWidth)
	if contentWidth == width {
		return lines
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = line + " "
	}
	return out
}

// LayoutNode ports upstream ScrollView[LAYOUT_NODE](): a scroll node whose state
// is this ScrollView narrowed to ScrollLayoutState.
func (s *ScrollView) LayoutNode() LayoutNode {
	return ScrollLayoutNode{Component: s.child, State: s}
}

// Dispose stops the hide timer so no goroutine outlives the view.
func (s *ScrollView) Dispose() {
	s.mu.Lock()
	s.stopHideTimer()
	s.mu.Unlock()
}

// Compile-time proof ScrollView satisfies the layout contracts.
var (
	_ ScrollLayoutState = (*ScrollView)(nil)
	_ LayoutComponent   = (*ScrollView)(nil)
)
