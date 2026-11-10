package tui

import (
	"errors"
	"fmt"
	"slices"
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
		// Mirror the host's write guard, so a hub test cannot pass by saving a cycle
		// the settings manager would refuse.
		if alias, ok := modelroles.RoleRef(value); ok && modelroles.AliasReaches(config, alias, role) {
			return errors.New("model role cycle")
		}
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

// browserRoles returns the role names the browser is currently offering, and parks
// the cursor on want when it is among them.
func browserRoles(h *ModelHub, want string) []string {
	roles := []string{}
	for i, item := range h.browser.items {
		if item.Role == "" {
			continue
		}
		roles = append(roles, item.Role)
		if item.Role == want {
			h.browser.cursor = i
		}
	}
	return roles
}

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

// distinctTokenTheme installs a theme whose every token resolves to a different
// color, so a rendered segment identifies the token that painted it. A source path
// is set because modelOverlayTheme hands back the active theme unchanged for an
// explicit custom theme, which keeps this fixture authoritative for both the
// catalog and anything that reads the catalog's palette.
func distinctTokenTheme(t *testing.T) *Theme {
	t.Helper()
	previous := ActiveTheme()
	t.Cleanup(func() { SetThemeInstance(previous) })
	theme := &Theme{Name: "distinct-tokens", SourcePath: "/fixture/distinct.json", mode: TerminalColorModeTrueColor, ownAppearance: "dark", fgAnsi: map[string]string{}, bgAnsi: map[string]string{}, dimTokens: map[string]bool{}}
	for i, token := range ThemeColorTokens() {
		// Distinct greys: 3 + i per channel keeps every token distinguishable.
		value := 3 + i%200
		theme.fgAnsi[token.Name] = fmt.Sprintf("\x1b[38;2;%d;%d;%dm", value, value, value)
		theme.bgAnsi[token.Name] = fmt.Sprintf("\x1b[48;2;%d;%d;%dm", value, value, value)
	}
	theme.populateFields()
	SetThemeInstance(theme)
	return theme
}

// TestModelHubCycleTrackColorsByRole diverges from OMP deliberately: OMP paints the
// cycle track from a positional palette, so the same role changes color when the
// order changes. This runtime colors each segment with the role's own catalog token
// instead, so the cycle reads as the same roles the manager rows, the quick picker
// and the footer show. The assertion is that a role's color is independent of its
// position, which is exactly what the positional palette could not provide.
func TestModelHubCycleTrackColorsByRole(t *testing.T) {
	theme := distinctTokenTheme(t)
	config := modelroles.Config{ModelRoles: map[string]string{"default": "p/a", "smol": "p/b", "slow": "p/c", "task": "@default"}}
	for _, tc := range []struct{ order []string }{
		{[]string{"default", "smol", "slow", "task"}},
		{[]string{"task"}},
		{[]string{"task", "default"}},
		{[]string{"slow", "task", "default"}},
	} {
		track, _ := modelRoleTrack(config, tc.order, -1)
		// Every role in the order draws in its own token, at any position.
		for _, role := range tc.order {
			token := config.RoleInfo(role).Color
			if !strings.Contains(track, theme.GetFgAnsi(token)+role) {
				t.Fatalf("order %v: role %s not drawn in %s: %q", tc.order, role, token, track)
			}
		}
	}
	// A role whose color comes from a modelTags override follows the tag, and an
	// undefined token falls back to muted instead of panicking in GetFgAnsi.
	config.ModelTags = map[string]modelroles.Tag{"slow": {Color: "error"}, "odd": {Color: "not-a-token"}}
	if track, _ := modelRoleTrack(config, []string{"slow"}, -1); !strings.Contains(track, theme.GetFgAnsi("error")+"slow") {
		t.Fatalf("tag override ignored: %q", track)
	}
	if track, _ := modelRoleTrack(config, []string{"odd"}, -1); !strings.Contains(track, theme.GetFgAnsi("muted")+"odd") {
		t.Fatalf("undefined token did not fall back: %q", track)
	}
	// The active segment keeps its chip treatment: caps around a padded bold label,
	// and the next segment is separated without a divider (OMP's active-spacing rule).
	active, ranges := modelRoleTrack(config, []string{"default", "task"}, 0)
	visible := widthx.StripTerminalSequences(active)
	if !strings.Contains(visible, "◀ default ▶") {
		t.Fatalf("active role chip missing: %q", visible)
	}
	if strings.Contains(visible, "┆") {
		t.Fatalf("active segment still drew a divider: %q", visible)
	}
	if len(ranges) != 2 || ranges[0][0] != 0 || ranges[1][0] <= ranges[0][0] {
		t.Fatalf("mouse ranges unusable for the chip geometry: %v", ranges)
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

// TestModelHubOffersRoleFallbackDestinations keeps the picker and the write guard
// in agreement: while a chain destination is being chosen, the browser lists @role
// rows ahead of models, the originating role and every role that would close a
// reference loop are absent, and accepting a role row writes the role name rather
// than the model the role resolves to today.
func TestModelHubOffersRoleFallbackDestinations(t *testing.T) {
	h, config, chains := hubFixture()
	config.ModelRoles["review"] = "p/a"
	config.ModelRoles["slow"] = "q/other"
	chains["slow"] = []string{"@review"}
	h.reload()
	openRoles(h)
	for i, row := range h.rows {
		if row.kind == "role" && row.key == "review" {
			h.cursor = i
		}
	}
	h.HandleInput("f")
	if h.target == nil || h.target.kind != "append" {
		t.Fatal("append did not start a chain pick")
	}
	h.HandleInput("@")
	roles := browserRoles(h, "default")
	if slices.Contains(roles, "review") || slices.Contains(roles, "slow") {
		t.Fatalf("loop-closing roles offered: %v", roles)
	}
	if !slices.Contains(roles, "default") {
		t.Fatalf("eligible role missing: %v", roles)
	}
	h.HandleInput("\r")
	if got := chains["review"]; !slices.Contains(got, "@default") {
		t.Fatalf("role destination saved as a model: %v", got)
	}
}

// TestModelHubAssignsRoleToRole covers primary assignment accepting a role alias.
// Assigning task to @default must save the alias, so task keeps tracking default
// when default is repointed, and the effort dial must not pin a model in its place.
func TestModelHubAssignsRoleToRole(t *testing.T) {
	h, config, _ := hubFixture()
	config.ModelRoles["smol"] = "p/b"
	h.reload()
	openRoles(h)
	for i, row := range h.rows {
		if row.kind == "role" && row.key == "task" {
			h.cursor = i
		}
	}
	h.HandleInput("\r")
	if h.target == nil || h.target.kind != "role" {
		t.Fatal("primary assignment does not accept roles")
	}
	// A leading @ is the alias gesture, mirroring the Alt+P picker.
	h.HandleInput("@")
	if !h.rolePool() {
		t.Fatal("@ did not open the alias pool")
	}
	roles := browserRoles(h, "default")
	if slices.Contains(roles, "task") {
		t.Fatalf("self assignment offered: %v", roles)
	}
	if !slices.Contains(roles, "default") {
		t.Fatalf("default not offered: %v", roles)
	}
	h.HandleInput("\r")
	if got := config.ModelRoles["task"]; got != "@default" {
		t.Fatalf("assignment pinned a model instead of the alias: %q", got)
	}
	// The alias must survive an effort change rather than collapsing to a model.
	h.reload()
	for i, row := range h.rows {
		if row.kind == "role" && row.key == "task" {
			h.cursor = i
		}
	}
	if item, _, ok := h.selectedRowItem(); !ok || item.Role != "default" {
		t.Fatalf("effort edit would pin the model: %+v", item)
	}
	// The loop direction: default must not be assignable to task.
	if got := modelroles.AssignableRoles(*config, nil, "default"); slices.Contains(got, "task") {
		t.Fatalf("cycle-closing alias offered: %v", got)
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

// TestModelHubRendersRoleAlias covers how a role-to-role assignment is displayed.
// The authored alias is the assignment, so the row shows @role in the referenced
// role's own color and must not substitute the model that role resolves to today;
// a chain entry naming a role renders the same way under its owner.
func TestModelHubRendersRoleAlias(t *testing.T) {
	h, config, chains := hubFixture()
	config.ModelRoles["task"] = "@default"
	config.ModelRoles["review"] = "@slow:high"
	config.ModelRoles["slow"] = "p/b"
	chains["task"] = []string{"@slow:off"}
	h.reload()
	openRoles(h)
	// Park the cursor on a row with no alias: the focused row paints its value in the
	// accent color, which would mask the role color this test is about.
	for i, row := range h.rows {
		if row.kind == "role" && row.key == "vision" {
			h.cursor = i
		}
	}
	frame := h.Render(100)
	rowLine := func(fragment string) string {
		for _, line := range frame {
			if strings.Contains(widthx.StripTerminalSequences(line), fragment) {
				return line
			}
		}
		return ""
	}
	theme := modelOverlayTheme()
	// Each alias renders as @name, colored by the role it names, with no model.
	for _, tc := range []struct{ role, alias, color string }{{"task", "@default", "success"}, {"review", "@slow", "accent"}} {
		line := rowLine(tc.alias)
		if line == "" {
			t.Fatalf("%s row does not show %s", tc.role, tc.alias)
		}
		if strings.Contains(widthx.StripTerminalSequences(line), "→ p/") {
			t.Fatalf("%s row substituted the resolved model: %q", tc.role, line)
		}
		if !strings.Contains(line, theme.Fg(tc.color, tc.alias)) {
			t.Fatalf("%s alias is not colored %s: %q", tc.role, tc.color, line)
		}
	}
	// A chain entry naming a role shows the role, not the model behind it.
	line := rowLine("↳ @slow")
	if line == "" || !strings.Contains(line, theme.Fg("accent", "@slow")) {
		t.Fatalf("fallback role destination is not a colored @role: %q", line)
	}
	if !strings.Contains(widthx.StripTerminalSequences(line), "off") {
		t.Fatalf("fallback role suffix lost: %q", line)
	}
}

// TestModelHubRoleHintKeepsCycleAction asserts that the Roles footer still
// advertises `c` on a terminal where the row's optional delete/thinking/
// compaction actions do not fit: those tokens are longer than the mandatory
// keys that follow them, and the frame truncates the footer from the right.
func TestModelHubRoleHintKeepsCycleAction(t *testing.T) {
	h, config, _ := hubFixture()
	reasoner := metricTestModel("a", func(m *ai.Model) {
		m.ProviderMeta.Reasoning = true
		m.Capabilities.MaxThinking = "high"
	})
	h.cfg.Models = []ModelSelectorItem{{Provider: "p", ID: "a", Model: reasoner}}
	h.cfg.AllModels = h.cfg.Models
	h.cfg.Compaction = func(string, string, bool) (string, bool, error) { return "", false, nil }
	h.cfg.DeleteRole = func(string, string) error { return nil }
	config.ModelRoles["review"] = "p/a"
	config.ModelTags = map[string]modelroles.Tag{"review": {Name: "review"}}
	h.result(nil)
	openRoles(h)
	for i, row := range h.rows {
		if row.kind == "role" && row.key == "review" {
			h.cursor = i
		}
	}
	// The full hint is wider than 80 columns once every optional action applies.
	full := h.footer(1000)
	for _, action := range []string{"D delete role", "t thinking", "k compaction limit", "c cycle"} {
		if !strings.Contains(full, action) {
			t.Fatalf("fixture is missing %q: %q", action, full)
		}
	}
	if widthx.VisibleWidth(full) <= 120 {
		t.Fatalf("fixture does not exceed a narrow footer: %q", full)
	}
	for _, width := range []int{60, 70, 80, 90, 100, 120} {
		lines := h.Render(width)
		footer := widthx.StripTerminalSequences(lines[len(lines)-2])
		if !strings.Contains(footer, "c cycle") {
			t.Fatalf("width %d: cycle action missing from %q", width, footer)
		}
	}
	for _, width := range []int{80, 90, 100, 120} {
		lines := h.Render(width)
		if footer := widthx.StripTerminalSequences(lines[len(lines)-2]); !strings.Contains(footer, "n new") {
			t.Fatalf("width %d: creation action missing from %q", width, footer)
		}
	}
	lines := h.Render(160)
	if footer := widthx.StripTerminalSequences(lines[len(lines)-2]); !strings.Contains(footer, "D delete role") || !strings.Contains(footer, "t thinking") || !strings.Contains(footer, "k compaction limit") {
		t.Fatalf("wide footer lost available actions: %q", footer)
	}
}
