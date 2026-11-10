package tui

import (
	"maps"
	"sync"
)

var modelOverlayThemes sync.Map

// ModelOverlayTheme returns the theme the model catalog paints with, which is the
// active theme for a light or explicit custom theme and OMP's Titanium palette for
// the inherited stock dark/system themes. Anything that must show a role in the
// catalog's colors has to read them here rather than from ActiveTheme: on the
// stock dark theme those are two different palettes, so a role painted from
// ActiveTheme reads as a different hue than the same role in the manager.
func ModelOverlayTheme() *Theme { return modelOverlayTheme() }

// modelOverlayTheme uses OMP's Titanium chrome for the inherited stock dark
// theme. Reusing PiG's stock palette made even correctly laid-out ports visibly
// different (purple selection and blue borders instead of blue/charcoal).
// Explicit custom/light themes remain authoritative. Cached immutable copies
// never mutate the transcript palette or allocate per model row. Values are
// translated from OMP's theme/defaults.json, not sampled approximation colors.
func modelOverlayTheme() *Theme {
	active := ActiveTheme()
	if (active.Name != "dark" && active.Name != "system") || active.Appearance() == "light" || active.SourcePath != "" {
		return active
	}
	if cached, ok := modelOverlayThemes.Load(active); ok {
		return cached.(*Theme)
	}
	// Theme contains an atomic color-resolution cache: copying the struct
	// would copy synchronization state. This private rendering-only projection
	// needs token tables and appearance, not the transcript's concrete cache.
	out := &Theme{Name: active.Name, SourceInfo: active.SourceInfo, SourcePath: active.SourcePath, mode: active.GetColorMode(), ownAppearance: active.Appearance(), fgAnsi: maps.Clone(active.fgAnsi), bgAnsi: maps.Clone(active.bgAnsi), dimTokens: maps.Clone(active.dimTokens)}
	colors := map[string]string{"accent": "#00b4ff", "border": "#2a3038", "success": "#00ff88", "error": "#ff4757", "warning": "#ffb347", "muted": "#9ca3b0", "dim": "#6b7280", "mdLink": "#00b4ff", "mdCode": "#00ff88", "syntaxKeyword": "#00b4ff", "syntaxFunction": "#00ff88", "syntaxVariable": "#e8ecf4", "syntaxString": "#d4c090", "syntaxNumber": "#ffb347", "syntaxOperator": "#00b4ff", "statusLineSep": "#2a3038"}
	for token, hex := range colors {
		color, _ := ParseColor(hex)
		out.fgAnsi[token] = ForegroundAnsi(color, active.GetColorMode())
		out.bgAnsi[token] = BackgroundAnsi(color, active.GetColorMode())
		delete(out.dimTokens, token)
	}
	selected, _ := ParseColor("#0082b3")
	out.bgAnsi["selectedBg"] = BackgroundAnsi(selected, active.GetColorMode())
	out.populateFields()
	cached, _ := modelOverlayThemes.LoadOrStore(active, out)
	return cached.(*Theme)
}
