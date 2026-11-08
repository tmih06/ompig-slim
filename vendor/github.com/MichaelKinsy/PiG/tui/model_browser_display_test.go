package tui

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestBrowserPreferredSeparatorKeepsNavigationValid covers the real catalog
// shape where current/role models sit above a separator and unassigned models.
func TestBrowserPreferredSeparatorKeepsNavigationValid(t *testing.T) {
	b := NewModelBrowser("p/current")
	b.roleConfig = modelroles.Config{ModelRoles: map[string]string{"review": "p/review"}}
	b.SetRoles(map[string]string{"review": "p/review"})
	b.SetItems([]ModelSelectorItem{{Provider: "p", ID: "other"}, {Provider: "p", ID: "review"}, {Provider: "p", ID: "current"}})
	if b.items[0].ID != "current" || b.items[1].ID != "review" || b.items[2].Kind != "separator" || b.items[3].ID != "other" {
		t.Fatal(b.items)
	}
	b.Move(1, true)
	b.Move(1, true)
	if b.Selected() == nil || b.Selected().ID != "other" {
		t.Fatal("keyboard landed on separator")
	}
	b.Move(-1, true)
	if b.Selected().ID != "review" {
		t.Fatal("reverse navigation landed on separator")
	}
	b.SetQuery("other")
	if len(b.filtered) != 1 {
		t.Fatal("separator survived search", b.filtered)
	}
}

// TestTaskPickerMarksEffectiveOverride checks that toggling targets updates the
// current marker, row preselection, and role-search return path together.
func TestTaskPickerMarksEffectiveOverride(t *testing.T) {
	items, config := modelReferenceFixture()
	p := NewModelPicker(ModelPickerConfig{Models: items, RoleConfig: config, Current: "fixture/alpha", TaskSelector: "fixture/beta", TaskKeys: []string{"alt+p"}})
	p.HandleInput("\x1bp")
	if p.browser.current != "fixture/beta" || p.browser.Selected().ID != "beta" {
		t.Fatal("task override not preselected")
	}
	p.SetFilter("@")
	p.SetFilter("")
	if p.browser.current != "fixture/beta" {
		t.Fatal("role search restored session instead of task marker")
	}
	p.HandleInput("\x1bp")
	if p.browser.current != "fixture/alpha" || p.browser.Selected().ID != "alpha" {
		t.Fatal("session target not restored")
	}
}

// TestBrowserMetricsOmitLegacyEfforts protects OMP's compact context/price
// formatting without changing the generic PiG metric API used elsewhere.
func TestBrowserMetricsOmitLegacyEfforts(t *testing.T) {
	items, _ := modelReferenceFixture()
	b := NewModelBrowser("")
	b.SetItems(items)
	lines := b.Render(100, 12)
	row := widthx.StripTerminalSequences(lines[2])
	if !strings.Contains(row, "128k ◫") || strings.Contains(row, " ctx") {
		t.Fatal(row)
	}
	facts := browserFacts(items[0])
	if !strings.Contains(facts, "16k out") || !strings.Contains(facts, "$2.5/10 per M") || strings.Contains(facts, "cache read") {
		t.Fatal(facts)
	}
}
