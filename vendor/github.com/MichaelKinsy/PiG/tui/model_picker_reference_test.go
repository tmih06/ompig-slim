package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// modelReferenceFixture mirrors the isolated catalog used by the real OMP PTY
// comparison. No production catalog values or credentials influence snapshots.
func modelReferenceFixture() ([]ModelSelectorItem, modelroles.Config) {
	alpha := &ai.Model{ID: "alpha", DisplayName: "Alpha", Input: []string{"text", "image"}, Capabilities: ai.ModelCapabilities{ContextWindow: 128000, MaxOutputTokens: 16000, InputCostPer1M: 2.5, OutputCostPer1M: 10}}
	beta := &ai.Model{ID: "beta", DisplayName: "Beta", Input: []string{"text"}, Capabilities: ai.ModelCapabilities{ContextWindow: 64000, MaxOutputTokens: 8000}}
	return []ModelSelectorItem{{Provider: "fixture", ID: "alpha", Name: "Alpha", Model: alpha}, {Provider: "fixture", ID: "beta", Name: "Beta", Model: beta}}, modelroles.Config{ModelRoles: map[string]string{"default": "fixture/alpha:high", "review": "fixture/beta"}, CycleOrder: []string{"default", "review"}}
}

// assertOMPCapture checks every display cell, including right padding/borders.
// The separate real-PTY comparator additionally checks foreground/background and
// bold on nonblank cells, which a text-only golden intentionally cannot assert.
func assertOMPCapture(t *testing.T, lines []string, width, height int, view string) {
	t.Helper()
	for i, line := range lines {
		lines[i] = widthx.StripTerminalSequences(line)
	}
	got := strings.Join(lines, "\n") + "\n"
	path := fmt.Sprintf("testdata/omp-18.14.2-%dx%d-%s.json", width, height, view)
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var expected []string
	if err := json.Unmarshal(want, &expected); err != nil {
		t.Fatal(err)
	}
	if got != strings.Join(expected, "\n")+"\n" {
		for i, line := range lines {
			if i >= len(expected) {
				t.Errorf("unexpected row %d: %q", i, line)
			} else if line != expected[i] {
				t.Errorf("row %d:\nwant %q\n got %q", i, expected[i], line)
			}
		}
		t.FailNow()
	}
}

// TestModelPickerMatchesOMPCapture compares real OMP 18.14.2 captures of the
// session, quick-role, and task sheets at full and narrow terminal geometries.
func TestModelPickerMatchesOMPCapture(t *testing.T) {
	items, config := modelReferenceFixture()
	roles := append([]ModelSelectorItem{}, items...)
	roles[0].Role = "default"
	roles[1].Role = "review"
	resolved := map[string]string{"default": "fixture/alpha:high", "review": "fixture/beta"}
	for _, role := range []string{"smol", "slow", "tiny", "memory", "widget", "judge"} {
		resolved[role] = "fixture/alpha"
	}
	for _, size := range [][2]int{{120, 40}, {80, 24}} {
		width, height := size[0], size[1]
		for _, view := range []string{"picker", "quick-roles", "task"} {
			t.Run(fmt.Sprintf("%dx%d/%s", width, height, view), func(t *testing.T) {
				p := NewModelPicker(ModelPickerConfig{Models: items, QuickRoles: roles, RoleConfig: config, ResolvedRoles: resolved, Current: "fixture/alpha", CurrentRole: "default", Height: func() int { return height }, TaskKeys: []string{"alt+p"}})
				switch view {
				case "quick-roles":
					p.SetFilter("@")
				case "task":
					p.HandleInput("\x1bp")
				}
				assertOMPCapture(t, p.Render(width), width, height, view)
			})
		}
	}
}

// TestModelHubMatchesOMPCapture covers entire fullscreen frames rather than
// isolated text fragments. Narrow Roles captures exercise overflow budgeting;
// provider/assignment captures catch hidden-prefix and focus-dependent hints.
func TestModelHubMatchesOMPCapture(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}} {
		width, height := size[0], size[1]
		for _, view := range []string{"hub", "provider", "roles", "assignment"} {
			t.Run(fmt.Sprintf("%dx%d/%s", width, height, view), func(t *testing.T) {
				items, config := modelReferenceFixture()
				pool := []modelroles.Candidate{{Provider: "fixture", ID: "alpha"}, {Provider: "fixture", ID: "beta"}}
				h := NewModelHub(ModelHubConfig{Models: items, Scoped: true, Current: "fixture/alpha", Height: func() int { return height }, DefaultThinking: func() string { return "high" }, Snapshot: func() (modelroles.Config, map[string][]string) { return config, nil }, Resolve: func(selector string) (string, error) { return config.ResolveAvailable(selector, "fixture/alpha", pool) }})
				switch view {
				case "provider":
					h.HandleInput("\x1b[B")
				case "roles":
					h.HandleInput("\x1b[A")
				case "assignment":
					openRoles(h)
					h.HandleInput("\r")
				}
				assertOMPCapture(t, h.Render(width), width, height, view)
			})
		}
	}
}
