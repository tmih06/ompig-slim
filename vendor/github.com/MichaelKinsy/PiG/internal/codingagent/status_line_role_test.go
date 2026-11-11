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
// a test, so a segment's color identifies the token that drew it. It has a source
// path, so tui.ModelOverlayTheme returns it unchanged; that makes this fixture
// authoritative for both the catalog and the footer, which is what a test about
// token identity needs. TestFooterRoleMatchesCatalogOnStockDarkTheme covers the
// case where the two themes genuinely differ.
func footerFixtureTheme(t *testing.T) *tui.Theme {
	t.Helper()
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	theme := footerTheme(t)
	tui.SetThemeInstance(theme)
	return theme
}

// footerRawSelectionLine returns the footer row that carries the model, with its
// escape sequences intact, which is what a color assertion needs. That row is line
// 1, beside the project, so a narrow terminal shows both at once.
func footerRawSelectionLine(t *testing.T, role string) string {
	t.Helper()
	return footerRoleLine(t, role, "")
}

// footerRoleLine renders the model row for a role and the color token its metadata
// reports, which is what FooterSession supplies at runtime.
func footerRoleLine(t *testing.T, role, color string) string {
	t.Helper()
	footer := NewFooterComponent(roleFooterModel(), "agent", nil)
	footer.SetThinkingLevel("high")
	footer.activeRole = func() (string, string) { return role, color }
	lines := footer.Render(120)
	if len(lines) < 2 {
		t.Fatalf("footer rendered %d lines, want at least 2", len(lines))
	}
	return lines[0]
}

// footerSelectionLine returns the model row as visible text.
func footerSelectionLine(t *testing.T, role string) string {
	t.Helper()
	return widthx.StripTerminalSequences(footerRawSelectionLine(t, role))
}

// TestFooterShowsActiveRoleBesideModel covers the reason the role is displayed at
// all: Ctrl+P cycling and fallback recovery can leave two different roles on the
// same physical model, so the model name alone cannot say which one is active. It
// also checks the role is not drawn in the model's own color, because a role
// painted like the model would not read as a distinction.
func TestFooterShowsActiveRoleBesideModel(t *testing.T) {
	theme := footerFixtureTheme(t)
	for _, role := range []string{"default", "review", "my-custom_role"} {
		line := footerSelectionLine(t, role)
		if want := "@" + role + " claude-sonnet-4 • high"; !strings.Contains(line, want) {
			t.Fatalf("role %q not shown beside the model: %q", role, line)
		}
	}
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

// TestFooterRoleMatchesCatalogOnStockDarkTheme is the regression this guards. On
// the inherited stock dark (and system) themes the model catalog paints with OMP's
// Titanium palette while the transcript keeps PiG's stock colors, so reading the
// active theme painted every role in the footer a different hue than the same role
// in the manager. The footer must take the catalog's palette instead.
func TestFooterRoleMatchesCatalogOnStockDarkTheme(t *testing.T) {
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	dark, err := tui.LoadBuiltinTheme("dark")
	if err != nil {
		t.Fatalf("load stock dark theme: %v", err)
	}
	// Pin truecolor. BuiltinTheme resolves in the terminal's mode, and a runner
	// that answers no color query resolves 256-color, where quantization maps the
	// two palettes' greys onto the same index. That made this test pass on a
	// truecolor developer terminal and fail in CI for a reason unrelated to the
	// behavior under test.
	dark = dark.WithColorMode(tui.TerminalColorModeTrueColor)
	tui.SetThemeInstance(dark)
	catalog, active := tui.ModelOverlayTheme(), tui.ActiveTheme()
	config := modelroles.Config{ModelRoles: map[string]string{"default": "p/a"}}

	// The fixture must actually reproduce the divergence, or this test would pass
	// on a runtime that no longer has two palettes.
	if catalog.GetFgAnsi("success") == active.GetFgAnsi("success") {
		t.Skip("stock dark theme no longer uses a separate catalog palette")
	}
	for _, role := range []string{"default", "smol", "slow", "task"} {
		token := config.RoleInfo(role).Color
		line := footerRoleLine(t, role, token)
		if !strings.Contains(line, catalog.GetFgAnsi(token)+"@"+role) {
			t.Fatalf("role %s is not drawn in the catalog's %s: %q", role, token, line)
		}
		// Only assert the negative when the two palettes actually differ for this
		// token: distinct tokens can still share an indexed color after
		// quantization, and a shared color is not a wrong color.
		if activeAnsi := active.GetFgAnsi(token); activeAnsi != catalog.GetFgAnsi(token) &&
			strings.Contains(line, activeAnsi+"@"+role) {
			t.Fatalf("role %s still uses the transcript palette: %q", role, line)
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
	footer.activeRole = func() (string, string) { return role, "" }
	if line := widthx.StripTerminalSequences(footer.Render(120)[0]); !strings.Contains(line, "@default") {
		t.Fatalf("initial role missing: %q", line)
	}
	role = "review"
	if line := widthx.StripTerminalSequences(footer.Render(120)[0]); !strings.Contains(line, "@review") {
		t.Fatalf("role change not reflected: %q", line)
	}
}
