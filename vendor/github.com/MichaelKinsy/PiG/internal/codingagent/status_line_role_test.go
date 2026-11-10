package codingagent

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// roleFooterModel is a fixture chat model with a reasoning ladder, so the footer
// renders its effort suffix as well as its name.
func roleFooterModel() *ai.Model {
	return &ai.Model{
		ID:           "claude-sonnet-4",
		ProviderMeta: ai.ProviderMetadata{ProviderID: "anthropic", Reasoning: true},
		Capabilities: ai.ModelCapabilities{MaxThinking: "high", ContextWindow: 200000},
	}
}

// footerFixtureTheme installs the upstream distinct-token theme for the duration of
// a test, so a segment's color identifies the token that drew it.
func footerFixtureTheme(t *testing.T) *tui.Theme {
	t.Helper()
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	theme := footerTheme(t)
	tui.SetThemeInstance(theme)
	return theme
}

// footerRawSelectionLine returns line 2 of the footer with its escape sequences
// intact, which is what a color assertion needs.
func footerRawSelectionLine(t *testing.T, role string) string {
	t.Helper()
	return footerRoleLine(t, role, "")
}

// footerRoleLine renders line 2 for a role and the color token its metadata
// reports, which is what FooterSession supplies at runtime.
func footerRoleLine(t *testing.T, role, color string) string {
	t.Helper()
	footer := NewFooterComponent(roleFooterModel(), "agent", nil)
	footer.SetThinkingLevel("high")
	footer.activeRole = func() string { return role }
	footer.activeRoleColor = func() string { return color }
	lines := footer.Render(120)
	if len(lines) < 2 {
		t.Fatalf("footer rendered %d lines, want at least 2", len(lines))
	}
	return lines[1]
}

// footerSelectionLine returns line 2 of the footer as visible text.
func footerSelectionLine(t *testing.T, role string) string {
	t.Helper()
	return widthx.StripTerminalSequences(footerRawSelectionLine(t, role))
}

// TestFooterShowsActiveRoleBesideModel covers the reason the role is displayed at
// all: Ctrl+P cycling and fallback recovery can leave two different roles on the
// same physical model, so the model name alone cannot say which one is active.
func TestFooterShowsActiveRoleBesideModel(t *testing.T) {
	theme := footerFixtureTheme(t)
	for _, role := range []string{"default", "review", "my-custom_role"} {
		line := footerSelectionLine(t, role)
		want := "@" + role + " claude-sonnet-4 • high"
		if !strings.Contains(line, want) {
			t.Fatalf("role %q not shown beside the model: %q", role, line)
		}
	}
	// Two roles that resolve to one model must be distinguishable.
	if first, second := footerSelectionLine(t, "default"), footerSelectionLine(t, "review"); first == second {
		t.Fatalf("shared-model roles render identically: %q", first)
	}
	// The model keeps its own token, and the role must not be drawn in it: a role
	// colored like the model would be invisible as a distinction. The raw line is
	// checked because stripping escapes hides which token colored what.
	raw := footerRawSelectionLine(t, "review")
	if !strings.Contains(raw, theme.GetFgAnsi("statusLineModel")+"claude-sonnet-4") {
		t.Fatalf("model is not drawn with statusLineModel: %q", raw)
	}
	if strings.Contains(raw, theme.GetFgAnsi("statusLineModel")+"@review") {
		t.Fatalf("role is drawn in the model's color: %q", raw)
	}
}

// TestFooterRoleUsesCatalogColor is the parity requirement: a role must read the
// same hue in the footer as in the model manager and catalog, which both paint
// from modelroles.Config.RoleInfo. The cases below are the ones that differ in
// practice, including a modelTags override and a token the theme does not define.
func TestFooterRoleUsesCatalogColor(t *testing.T) {
	theme := footerFixtureTheme(t)
	config := modelroles.Config{
		ModelRoles: map[string]string{"default": "p/a", "review": "p/b"},
		// review overrides its built-in error hue; an unknown token is accepted by
		// settings, so the footer must not take the panicking Fg path with it.
		ModelTags: map[string]modelroles.Tag{"review": {Color: "accent"}, "broken": {Color: "not-a-token"}},
	}
	cases := []struct{ role, wantToken string }{
		{"default", "success"},
		{"smol", "warning"},
		{"slow", "accent"},
		{"review", "accent"},            // the tag override, not the built-in error
		{"task", "muted"},               // a custom-row default
		{"broken", "statusLineContext"}, // unknown token falls back
	}
	for _, tc := range cases {
		color := config.RoleInfo(tc.role).Color
		line := footerRoleLine(t, tc.role, color)
		if !strings.Contains(line, theme.GetFgAnsi(tc.wantToken)+"@"+tc.role) {
			t.Fatalf("role %s (catalog color %q) not drawn with %s: %q", tc.role, color, tc.wantToken, line)
		}
	}
}

// TestFooterOmitsRoleWhenUnset keeps a session without role identity byte-identical
// to upstream: a direct model selection and a virtual model render no role.
func TestFooterOmitsRoleWhenUnset(t *testing.T) {
	line := footerSelectionLine(t, "")
	if !strings.Contains(line, "claude-sonnet-4 • high") {
		t.Fatalf("model line missing: %q", line)
	}
	if strings.Contains(line, "@") {
		t.Fatalf("unset role rendered a marker: %q", line)
	}
	// A footer never given a session has no role source at all.
	footer := NewFooterComponent(roleFooterModel(), "agent", nil)
	lines := footer.Render(120)
	for _, candidate := range lines {
		if strings.Contains(widthx.StripTerminalSequences(candidate), "@") {
			t.Fatalf("unbound footer rendered a role: %q", candidate)
		}
	}
}

// TestFooterFollowsRoleChangesLive pins the read-on-render contract: a role can
// change without the physical model changing, so a stale cached value would show
// the wrong role until an unrelated model rebind.
func TestFooterFollowsRoleChangesLive(t *testing.T) {
	footer := NewFooterComponent(roleFooterModel(), "agent", nil)
	role := "default"
	footer.activeRole = func() string { return role }
	if line := widthx.StripTerminalSequences(footer.Render(120)[1]); !strings.Contains(line, "@default") {
		t.Fatalf("initial role missing: %q", line)
	}
	role = "review"
	if line := widthx.StripTerminalSequences(footer.Render(120)[1]); !strings.Contains(line, "@review") {
		t.Fatalf("role change not reflected: %q", line)
	}
}
