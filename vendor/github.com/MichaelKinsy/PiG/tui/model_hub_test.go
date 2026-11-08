package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// hubFixture mirrors OMP's cached catalog/settings host without paid model calls.
func hubFixture() (*ModelHub, *modelroles.Config, map[string][]string) {
	config := modelroles.Config{ModelRoles: map[string]string{"default": "p/a"}}
	chains := map[string][]string{"default": {"p/b", "p/c"}}
	items := []ModelSelectorItem{{Provider: "p", ID: "a"}, {Provider: "p", ID: "b"}, {Provider: "q", ID: "other"}}
	h := NewModelHub(ModelHubConfig{Models: items, AllModels: append(append([]ModelSelectorItem{}, items...), ModelSelectorItem{Provider: "locked", ID: "secret"}), Height: func() int { return 32 }, Snapshot: func() (modelroles.Config, map[string][]string) { return config.Clone(), modelroles.CloneChains(chains) }, Resolve: func(s string) (string, error) { return config.Resolve(s, "p/a") }, Assign: func(role, value, scope string) error {
		if value == "" {
			delete(config.ModelRoles, role)
		} else {
			config.ModelRoles[role] = value
		}
		return nil
	}, Fallback: func(key string, chain []string, scope string) error {
		if chain == nil {
			delete(chains, key)
		} else {
			chains[key] = chain
		}
		return nil
	}, Cycle: func(order []string, scope string) error { config.CycleOrder = order; return nil }})
	return h, &config, chains
}

// openRoles follows OMP's initial All/scope focus with Up then Right.
func openRoles(h *ModelHub) { h.HandleInput("\x1b[A"); h.HandleInput("\x1b[C") }

// TestModelHubEdits covers assign, append, delete, membership and ordered movement
// using upstream's x/f/c/[/] bindings instead of the abandoned approximate menu.
func TestModelHubEdits(t *testing.T) {
	h, c, chains := hubFixture()
	openRoles(h)
	h.HandleInput("\r")
	if h.target == nil || h.sidebarFocus {
		t.Fatal("assignment did not focus embedded rows")
	}
	h.HandleInput("\r")
	if c.ModelRoles["default"] != "p/a" || h.target != nil {
		t.Fatal(c.ModelRoles)
	}
	h.HandleInput("f")
	h.HandleInput("\r")
	if len(chains["default"]) != 3 || chains["default"][2] != "p/a" {
		t.Fatal(chains)
	}
	h.cursor = 1
	h.HandleInput("]")
	if chains["default"][1] != "p/b" || h.cursor != 2 {
		t.Fatal("fallback cursor did not follow", chains, h.cursor)
	}
	h.HandleInput("x")
	if len(chains["default"]) != 2 {
		t.Fatal(chains)
	}
	h.cursor = 0
	h.HandleInput("c")
	if len(c.CycleOrder) != 2 || c.CycleOrder[0] != "smol" {
		t.Fatal(c.CycleOrder)
	}
	h.HandleInput("c")
	h.HandleInput("[")
	if c.CycleOrder[1] != "default" {
		t.Fatal("cycle order", c.CycleOrder)
	}
}

// TestModelHubCreationAndFailure keeps invalid input editable and exposes disk errors.
func TestModelHubCreationAndFailure(t *testing.T) {
	h, c, _ := hubFixture()
	openRoles(h)
	h.HandleInput("n")
	h.HandleInput("review")
	h.HandleInput("\r")
	h.HandleInput("\r")
	if c.ModelRoles["review"] != "p/a" {
		t.Fatal(c.ModelRoles)
	}
	h.cfg.Assign = func(string, string, string) error { return errors.New("disk full") }
	h.setSide(0)
	h.cursor = 0
	h.sidebarFocus = false
	h.HandleInput("x")
	if !strings.Contains(strings.Join(h.Render(100), "\n"), "disk full") {
		t.Fatal("failure hidden")
	}
	for _, width := range []int{12, 36, 80, 160} {
		for _, line := range h.Render(width) {
			if widthx.VisibleWidth(line) > width {
				t.Fatalf("width %d: %q", width, line)
			}
		}
	}
}

// TestModelHubSearchAndLockedProvider verifies scope-local search and Escape's ladder.
func TestModelHubSearchAndLockedProvider(t *testing.T) {
	h, _, _ := hubFixture()
	h.HandleInput("other")
	if h.sidebarFocus || h.browser.Selected() == nil || h.browser.Selected().Provider != "q" {
		t.Fatal("typing did not focus matches")
	}
	h.HandleInput("\x1b")
	if h.Done() || h.browser.Query() != "" {
		t.Fatal("Escape closed before clearing query")
	}
	login := ""
	h.cfg.Login = func(provider string) { login = provider }
	for i, e := range h.entries {
		if e.locked {
			h.setSide(i)
			break
		}
	}
	h.HandleInput("\r")
	if login != "locked" {
		t.Fatal(login)
	}
	h.HandleInput("other")
	if h.entries[h.side].id != "all" {
		t.Fatal("locked provider captured search")
	}
}

// TestModelHubInlineRoleAndThinking covers async settlement and inert confirmation.
func TestModelHubInlineRoleAndThinking(t *testing.T) {
	h, c, _ := hubFixture()
	reasoner := metricTestModel("reasoner", func(m *ai.Model) { m.ProviderMeta.Reasoning = true; m.Capabilities.MaxThinking = "high" })
	h.cfg.Models = []ModelSelectorItem{{Provider: "deploy", ID: "reasoner", Model: reasoner}}
	h.cfg.AllModels = h.cfg.Models
	h.applyScope()
	h.sidebarFocus = false
	h.HandleInput("\r")
	if h.strip == nil || h.strip.kind != "roles" {
		t.Fatal("model activation skipped role strip")
	}
	var finish func(error)
	commits := 0
	h.cfg.AssignAsync = func(role, value, scope string, done func(error)) {
		commits++
		c.ModelRoles[role] = value
		finish = done
	}
	h.HandleInput("\r")
	if !h.pending || h.strip != nil {
		t.Fatal("async assignment not guarded")
	}
	h.HandleInput("\r")
	if commits != 1 {
		t.Fatal("duplicate async commit")
	}
	finish(nil)
	if h.strip == nil || h.strip.kind != "thinking" {
		t.Fatal("missing effort strip")
	}
	h.HandleInput("\r")
	if commits != 1 || h.strip != nil {
		t.Fatal("preselected effort was reapplied")
	}
}

// TestModelHubWheelDoesNotSelect verifies provider panning cannot change active scope.
func TestModelHubWheelDoesNotSelect(t *testing.T) {
	h, _, _ := hubFixture()
	h.Render(100)
	side := h.side
	cursor := h.browser.cursor
	h.HandleMouse(TuiMouseEvent{Type: MouseWheel, X: 2, Y: 3, WheelDelta: 1})
	if h.side != side {
		t.Fatal("sidebar wheel activated provider")
	}
	h.HandleMouse(TuiMouseEvent{Type: MouseWheel, X: 80, Y: 4, WheelDelta: 1})
	if h.browser.cursor != cursor {
		t.Fatal("wheel changed selection")
	}
}

// TestModelHubFallbackCreationDeduplicates checks the selected-model/provider key strip.
func TestModelHubFallbackCreationDeduplicates(t *testing.T) {
	h, _, chains := hubFixture()
	h.sidebarFocus = false
	h.HandleInput("\r")
	strip := h.strip
	if strip == nil {
		t.Fatal("no role strip")
	}
	for i, chip := range strip.chips {
		if chip.action == "providerChain" {
			strip.index = i
		}
	}
	h.HandleInput("\r")
	h.HandleInput("\r")
	if len(chains["p/*"]) != 1 {
		t.Fatal(chains)
	}
	h.commitChain("p/*", []string{"p/a", "p/a"}, "p/a")
	if len(chains["p/*"]) != 1 {
		t.Fatal("duplicate fallback", chains)
	}
}

// TestQuickPickerPreservesRole checks @ mode and Task/session isolation.
func TestQuickPickerPreservesRole(t *testing.T) {
	items := []ModelSelectorItem{{Provider: "p", ID: "a"}, {Provider: "p", ID: "b"}}
	roles := []ModelSelectorItem{{Provider: "p", ID: "a", Role: "review"}, {Provider: "p", ID: "a", Role: "default"}}
	picker := NewModelPicker(ModelPickerConfig{Models: items, QuickRoles: roles, TaskKeys: []string{"alt+p"}})
	picker.HandleInput("@review")
	picker.HandleInput("\r")
	if picker.SelectedFQ() != "@review" {
		t.Fatal(picker.SelectedFQ())
	}
	task := NewModelPicker(ModelPickerConfig{Models: items, TaskKeys: []string{"alt+p"}})
	task.HandleInput("\x1bp")
	task.HandleInput("\r")
	if !task.SelectedTask() {
		t.Fatal("shortcut did not toggle Task")
	}
}
