package tui

import (
	"github.com/MichaelKinsy/PiG/ai"
	"testing"
)

// TestBrowserOrderingMatchesOMP exercises catalog order independently of visual
// chrome: role rank, MRU, provider priority, versions, and revision dates.
func TestBrowserOrderingMatchesOMP(t *testing.T) {
	b := NewModelBrowser("")
	b.roles = map[string]string{"default": "p/gpt-4o"}
	b.recent = []string{"p/gpt-5.2"}
	priority := 1.0
	items := []ModelSelectorItem{{Provider: "p", ID: "gpt-4o-2024-05-13"}, {Provider: "p", ID: "gpt-5.4"}, {Provider: "p", ID: "gpt-4o-2024-11-20"}, {Provider: "p", ID: "gpt-4o"}, {Provider: "p", ID: "gpt-5.2"}, {Provider: "p", ID: "priority-first", Model: &ai.Model{Priority: &priority}}, {Provider: "p", ID: "gpt-4o-latest"}}
	b.SetItems(items)
	want := []string{"gpt-4o", "gpt-5.2", "", "priority-first", "gpt-4o-2024-11-20", "gpt-5.4", "gpt-4o-2024-05-13", "gpt-4o-latest"}
	for i, id := range want {
		if b.items[i].ID != id {
			t.Fatalf("slot %d got %q want %q", i, b.items[i].ID, id)
		}
	}
	b.SetQuery("gpt")
	if b.items[0].ID != "gpt-5.2" {
		t.Fatal("search kept role rank above MRU", b.items[0])
	}
	for _, entry := range []struct {
		id   string
		want float64
	}{{"gemini-2.5-pro", 2.5}, {"claude-sonnet-4-6", 4.6}, {"llama-3-1-8b", 3.1}, {"gpt-4o", 4}, {"plain", 0}} {
		if got := browserVersion(entry.id); got != entry.want {
			t.Fatal(entry.id, got)
		}
	}
}
