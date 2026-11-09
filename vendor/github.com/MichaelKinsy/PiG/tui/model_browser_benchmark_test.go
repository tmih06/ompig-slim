package tui

import (
	"runtime"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

// BenchmarkModelBrowser covers provider-sized and complete-catalog browsing:
// scope assignment, alternating fuzzy queries, and a 120x40 visible window.
// Catalog decoding is outside timing so sorting/search/rendering are attributable
// separately from loading. Resolved roles and MRU exercise real ordering rules.
func BenchmarkModelBrowser(b *testing.B) {
	for _, scope := range []string{"openai", "all"} {
		b.Run(scope, func(b *testing.B) {
			provider := scope
			if provider == "all" {
				provider = ""
			}
			rows := ai.ListModels(provider)
			items := make([]ModelSelectorItem, 0, len(rows))
			for _, row := range rows {
				items = append(items, ModelSelectorItem{Provider: row.Provider, ID: row.ID, Model: row.ToModel()})
			}
			browser := NewModelBrowser("openai/gpt-6.1-sol")
			browser.SetRoles(map[string]string{"default": "openai/gpt-6.1-sol:high", "review": "openai/gpt-4o", "vision": "openai/gpt-4o", "smol": "openai/gpt-6-luna"})
			browser.recent = []string{"openai/gpt-6.1-sol", "openai/gpt-4o", "openai/gpt-6-luna"}
			browser.SetItems(items)
			b.Run("SetItems", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					browser.SetItems(items)
				}
			})
			b.Run("Search", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if i%2 == 0 {
						browser.SetQuery("gpt")
					} else {
						browser.SetQuery("claude")
					}
				}
			})
			browser.SetQuery("")
			b.Run("Render", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					lines := browser.Render(120, 40)
					runtime.KeepAlive(lines)
				}
			})
		})
	}
}
