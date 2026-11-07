package tui

// Native presentation: keep the system theme useful on terminals without OSC
// color replies. The built-in palettes provide distinct, readable surfaces;
// terminals reporting their background still use the adaptive contrast solver.
func nativeFallbackThemeColors(appearance TerminalTheme) SystemThemeColors {
	palette := builtinDarkTheme()
	if appearance == "light" {
		palette = builtinLightTheme()
	}
	colors := make(map[string]ThemeColorValue, len(systemTokenFamilies))
	for _, entry := range systemTokenFamilies {
		colors[entry.token] = systemColorValue(ColorToHex(palette.concreteColors[entry.token]))
	}
	return SystemThemeColors{Colors: colors, Appearance: appearance}
}
