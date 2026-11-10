package tui

import (
	"fmt"
	"strings"
	"testing"
)

// BenchmarkInfoWidgetRender isolates host framing from provider rendering with
// cached rows. Long lists must not make a twelve-row viewport expensive to paint.
func BenchmarkInfoWidgetRender(b *testing.B) {
	for _, count := range []int{30, 300} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			body := make([]string, count)
			for i := range body {
				body[i] = fmt.Sprintf("☐ task %03d 界", i)
			}
			w := &InfoWidgets{Sections: []InfoWidget{{Key: "tasks", Title: "Tasks", Render: func(int, bool) []string { return body }}}}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w.Render(40)
			}
		})
	}
}

// BenchmarkInfoWidgetLayout measures complete cached transcript frames, including
// the final painter. Plain frames expose the overhead of visible and hidden HUDs.
func BenchmarkInfoWidgetLayout(b *testing.B) {
	for _, mode := range []string{"plain", "visible", "hidden"} {
		b.Run(mode, func(b *testing.B) {
			doc := NewPaddedText(strings.Repeat("response with 界 and text\n", 2000), 0, 0, nil)
			scroll := NewScrollView(doc, ScrollViewOptions{Follow: "end", Primary: true, Scrollbar: ScrollbarAlways})
			var root Component = scroll
			if mode != "plain" {
				w := widgetFixture()
				w.hidden = mode == "hidden"
				root = &InfoWidgetLayout{Content: scroll, Widgets: w}
			}
			RenderLayoutFrame(root, 120, 40, nil)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				RenderLayoutFrame(root, 120, 40, nil)
			}
		})
	}
}
