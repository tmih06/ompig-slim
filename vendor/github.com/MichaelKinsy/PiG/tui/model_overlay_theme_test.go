package tui

import (
	"maps"
	"testing"
)

// TestModelOverlayThemePreservesTranscript checks the OMP default palette is a
// detached projection, never a global transcript/theme mutation. Both render
// modes must remain valid after a user changes terminal color capabilities.
func TestModelOverlayThemePreservesTranscript(t *testing.T) {
	original := ActiveTheme()
	defer SetThemeInstance(original)
	for _, mode := range []TerminalColorMode{TerminalColorModeTrueColor, TerminalColorMode256} {
		stock := &Theme{Name: "dark", mode: mode, ownAppearance: "dark", fgAnsi: maps.Clone(original.fgAnsi), bgAnsi: maps.Clone(original.bgAnsi), dimTokens: maps.Clone(original.dimTokens)}
		stock.populateFields()
		SetThemeInstance(stock)
		active := ActiveTheme()
		before := active.Accent
		got := modelOverlayTheme()
		color, _ := ParseColor("#00b4ff")
		if got.Accent != ForegroundAnsi(color, active.GetColorMode()) {
			t.Fatalf("wrong OMP accent: %q", got.Accent)
		}
		if ActiveTheme() != active || active.Accent != before {
			t.Fatal("model rendering mutated the transcript theme")
		}
		if modelOverlayTheme() != got {
			t.Fatal("immutable projection was rebuilt")
		}
		// The track must use supported tokens even without OMP-specific theme keys.
		custom := &Theme{Name: "user-theme", SourcePath: "/fixture/theme.json", mode: mode, ownAppearance: "dark", fgAnsi: maps.Clone(active.fgAnsi), bgAnsi: maps.Clone(active.bgAnsi), dimTokens: maps.Clone(active.dimTokens)}
		custom.populateFields()
		SetThemeInstance(custom)
		if modelOverlayTheme() != ActiveTheme() {
			t.Fatal("custom theme was overridden")
		}
		modelSegmentTrack([]string{"all", "chat", "image"}, 0)
	}
}
