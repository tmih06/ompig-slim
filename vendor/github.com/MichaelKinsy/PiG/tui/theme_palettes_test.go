package tui

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// optionalThemePath resolves repository assets through the canonical test GOPATH
// symlink, so optional palettes are tested without embedding them in the binary.
func optionalThemePath(t *testing.T, name string) string {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	source, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(source), "../../../../../themes", name+".json")
}

// optionalPalette loads a user-installable palette through the normal file API.
func optionalPalette(t *testing.T, name string) *Theme {
	t.Helper()
	theme, err := LoadThemeFromPath(optionalThemePath(t, name), TerminalColorModeTrueColor)
	if err != nil {
		t.Fatal(err)
	}
	return theme
}

// TestOptionalPalettesLoad ensures every optional palette resolves every UI token.
func TestOptionalPalettesLoad(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(optionalThemePath(t, "titanium")), "*.json"))
	if err != nil || len(paths) != 100 {
		t.Fatalf("palettes=%d, err=%v", len(paths), err)
	}
	for _, path := range paths {
		theme := optionalPalette(t, strings.TrimSuffix(filepath.Base(path), ".json"))
		if !theme.HasANSIPalette() {
			t.Fatalf("%s has no ANSI palette", theme.Name)
		}
		for _, token := range ThemeColorTokens() {
			if themeBackgroundTokens[token.Name] {
				theme.Bg(token.Name, "x")
			} else {
				theme.Fg(token.Name, "x")
			}
		}
	}
}

// TestOnlyCoreThemesAreBuiltin keeps optional palettes out of the default picker
// while proving users can register one from disk without a special installer.
func TestOnlyCoreThemesAreBuiltin(t *testing.T) {
	registry := NewThemeRegistry()
	if got := registry.Names(); !slices.Equal(got, []string{"system", "dark", "light"}) {
		t.Fatalf("built-in themes: %v", got)
	}
	if registry.Get("titanium") != nil {
		t.Fatal("optional palette is built in")
	}
	theme := optionalPalette(t, "titanium")
	registry.AddFile(theme, theme.SourcePath)
	if registry.Get("titanium") != theme {
		t.Fatal("custom theme was not registered")
	}
}

// TestThemeSchemaAcceptsOMPSections covers the sections OMP writes that this
// engine does not render: a theme carrying them must load rather than be
// rejected, and a malformed one must still be reported.
func TestThemeSchemaAcceptsOMPSections(t *testing.T) {
	minimal := func() map[string]any {
		colors := map[string]any{}
		for _, token := range ThemeColorTokens() {
			if !token.Optional {
				colors[token.Name] = "#808080"
			}
		}
		return map[string]any{"name": "test", "colors": colors}
	}
	document := func(mutate func(map[string]any)) []byte {
		values := minimal()
		mutate(values)
		encoded, err := json.Marshal(values)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		return encoded
	}

	ansi := make([]any, ANSISlotCount)
	for i := range ansi {
		ansi[i] = "#101010"
	}
	for name, data := range map[string][]byte{
		"terminal and symbols": document(func(values map[string]any) {
			values["terminal"] = map[string]any{
				"background": "#101010", "foreground": "#e0e0e0", "chrome": "#141414",
				"widget": "#181818", "ansi": ansi,
			}
			values["symbols"] = map[string]any{
				"preset":        "unicode",
				"overrides":     map[string]any{"status.success": "✔"},
				"spinnerFrames": []any{"⠋", "⠙"},
			}
		}),
		// symbols is carried through untouched: this engine renders no glyph table of
		// its own, so the section is only checked to be an object.
		"symbols spinner object": document(func(values map[string]any) {
			values["symbols"] = map[string]any{"spinnerFrames": map[string]any{"status": []any{"⠋"}}}
		}),
	} {
		if err := ValidateThemeJSON(name, data); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	for name, data := range map[string][]byte{
		"terminal ansi wrong length": document(func(values map[string]any) {
			values["terminal"] = map[string]any{"ansi": []any{"#101010"}}
		}),
		"unknown terminal member": document(func(values map[string]any) {
			values["terminal"] = map[string]any{"chromeColor": "#101010"}
		}),
		"symbols not an object": document(func(values map[string]any) {
			values["symbols"] = []any{"unicode"}
		}),
		"terminal not an object": document(func(values map[string]any) {
			values["terminal"] = "#101010"
		}),
	} {
		if err := ValidateThemeJSON(name, data); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}

// TestThemeOptionalStatusLineTokens covers the fallbacks: a theme that predates
// the status line names no statusLine* token, and every one must still resolve
// to the token it borrows instead of panicking on the first footer render.
func TestThemeOptionalStatusLineTokens(t *testing.T) {
	foregrounds := []ThemeTokenValue{{Token: "accent", Value: ThemeColorValue{Text: "#00d7ff", isSet: true}}}
	for _, token := range themeColorTokens {
		if token.name == "accent" || themeBackgroundTokens[token.name] || token.name == "statusLineSep" || strings.HasPrefix(token.name, "statusLine") {
			continue
		}
		foregrounds = append(foregrounds, ThemeTokenValue{Token: token.name, Value: ThemeColorValue{Text: "#808080", isSet: true}})
	}
	backgrounds := []ThemeTokenValue{}
	for _, token := range themeColorTokens {
		if themeBackgroundTokens[token.name] {
			backgrounds = append(backgrounds, ThemeTokenValue{Token: token.name, Value: ThemeColorValue{Text: "#101010", isSet: true}})
		}
	}
	json := &ThemeJSON{
		Name:   "no-status-line",
		Colors: map[string]ThemeColorValue{},
		colorKeys: func() []string {
			keys := make([]string, 0, len(foregrounds)+len(backgrounds))
			for _, entry := range foregrounds {
				keys = append(keys, entry.Token)
			}
			for _, entry := range backgrounds {
				keys = append(keys, entry.Token)
			}
			return keys
		}(),
	}
	for _, entry := range foregrounds {
		json.Colors[entry.Token] = entry.Value
	}
	for _, entry := range backgrounds {
		json.Colors[entry.Token] = entry.Value
	}
	theme, err := resolveThemeWithMode(json, TerminalColorModeTrueColor)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	for token, want := range map[string]string{
		string(ThemeColorStatusLineModel):     theme.Accent,
		string(ThemeColorStatusLineSep):       theme.Dim,
		string(ThemeColorStatusLinePath):      theme.Border,
		string(ThemeColorStatusLineGitDirty):  theme.Warning,
		string(ThemeColorStatusLineUntracked): theme.Error,
		string(ThemeColorStatusLineSubagents): theme.Accent,
	} {
		got := theme.GetFgAnsi(token)
		if got != want {
			t.Fatalf("%s resolved to %q, want the borrowed token %q", token, got, want)
		}
	}
}

// TestANSIFgFallsBackToTerminalPalette covers a theme with no palette of its
// own: the slot still paints, using the terminal's ANSI color, because the
// terminal is guaranteed to render its own palette.
func TestANSIFgFallsBackToTerminalPalette(t *testing.T) {
	theme := paletteHueTheme(t)
	theme.ansiFg = nil
	if theme.HasANSIPalette() {
		t.Fatal("a theme without a palette reports one")
	}
	if got, want := theme.ANSIFg(ANSIGreen, "x"), "\x1b[32mx\x1b[39m"; got != want {
		t.Fatalf("ANSIFg fallback = %q, want %q", got, want)
	}
	theme.ansiFg = make([]string, ANSISlotCount)
	theme.ansiFg[ANSIBrightBlue] = "\x1b[38;2;1;2;3m"
	if got, want := theme.ANSIFg(ANSIBrightBlue, "x"), "\x1b[38;2;1;2;3mx\x1b[39m"; got != want {
		t.Fatalf("ANSIFg = %q, want %q", got, want)
	}
}

// TestDefaultThemeIsAPalette covers the out-of-the-box theme: a terminal with no
// stored setting gets a bundled palette rather than the generated system theme,
// which renders most tokens in the terminal's default foreground on a terminal
// that does not answer the color queries (which is what made the TUI look
// unstyled there).
func TestDefaultThemeIsAPalette(t *testing.T) {
	previousRegistry := ActiveThemeRegistry()
	previous := ActiveTheme()
	previousSetting := selectedThemeName.Load()
	t.Cleanup(func() {
		SetThemeRegistry(previousRegistry)
		SetThemeInstance(previous)
		selectedThemeName.Store(previousSetting)
	})
	SetThemeRegistry(NewThemeRegistry())

	if got := DefaultThemeName(TerminalColorSchemeDark); got != DefaultDarkTheme {
		t.Fatalf("dark default = %q", got)
	}
	if got := DefaultThemeName(TerminalColorSchemeLight); got != DefaultLightTheme {
		t.Fatalf("light default = %q", got)
	}
	// Set the appearance the way the process learns it, then take the no-setting
	// path: SetThemeSettingPresence(nil) must select that appearance's default. Asking
	// for the name directly would test DefaultThemeName against itself.
	for _, appearance := range []TerminalColorScheme{TerminalColorSchemeDark, TerminalColorSchemeLight} {
		SetTerminalColorScheme(appearance)
		SetThemeSettingPresence(nil)
		theme := ActiveTheme()
		if want := DefaultThemeName(appearance); theme.Name != want {
			t.Fatalf("%s: no setting selected %q, want %q", appearance, theme.Name, want)
		}
		if theme.Name == SystemThemeName {
			t.Fatalf("%s: default is still the system theme", appearance)
		}
		if !theme.HasANSIPalette() {
			t.Fatalf("%s: default %q has no ANSI palette", appearance, theme.Name)
		}
	}
	SetTerminalColorScheme("")
}

// TestDefaultThemeResolvesThroughCoreRegistry covers default selection and
// the system fallback for a theme the user has not installed.
func TestDefaultThemeResolvesThroughCoreRegistry(t *testing.T) {
	previousRegistry := ActiveThemeRegistry()
	previous := ActiveTheme()
	t.Cleanup(func() {
		SetThemeRegistry(previousRegistry)
		SetThemeInstance(previous)
	})
	bare := NewThemeRegistry()
	SetThemeRegistry(bare)

	SetThemeSettingPresence(nil)
	if name := ActiveTheme().Name; name != DefaultDarkTheme && name != DefaultLightTheme {
		t.Fatalf("default selected %q", name)
	}

	// A theme nothing knows about falls back rather than leaving no theme active.
	SetThemeByName("no-such-theme")
	if ActiveTheme() == nil {
		t.Fatal("no theme active after an unknown selection")
	}
	if name := ActiveTheme().Name; name != SystemThemeName {
		t.Fatalf("fallback selected %q, want the system theme", name)
	}
}

// TestANSIPaletteSurvivesColorMode covers the palette across a color-mode
// rebuild: activating a theme for a 256-color terminal re-resolves it from its
// JSON source, and the ANSI palette has to come back downsampled rather than
// missing, or headings would lose their hues on exactly the terminals that need
// the tree to be readable.
func TestANSIPaletteSurvivesColorMode(t *testing.T) {
	theme := optionalPalette(t, "titanium")
	if theme == nil {
		t.Fatal("titanium did not resolve")
	}
	truecolor := theme.WithColorMode(TerminalColorModeTrueColor)
	indexed := theme.WithColorMode(TerminalColorMode256)
	if !indexed.HasANSIPalette() {
		t.Fatal("the 256-color variant has no palette")
	}
	for slot := range ANSISlotCount {
		got := paletteSlotAnsi(indexed, slot)
		if got == "" {
			t.Fatalf("slot %d is unset in the 256-color variant", slot)
		}
		if strings.Contains(got, "\x1b[38;2;") {
			t.Fatalf("slot %d kept a truecolor sequence in the 256-color variant: %q", slot, got)
		}
		if want := paletteSlotAnsi(truecolor, slot); got == want {
			t.Fatalf("slot %d is identical in both modes: %q", slot, got)
		}
	}
}

// paletteSlotAnsi is the sequence ANSIFg opens for slot, so a test can compare
// the resolved colors instead of wrapped text.
func paletteSlotAnsi(theme *Theme, slot int) string {
	return strings.TrimSuffix(theme.ANSIFg(slot, ""), SGRFgReset)
}

// TestProseFg covers the assistant prose color. `text` is the terminal default in
// 96 of the bundled palettes, so prose that follows `text` inherits whatever the
// user's terminal shows; a palette records the body color it was designed against
// in terminal.foreground, which is what ProseFg falls back to.
// TestPaletteForegroundIsAdopted covers the body-color repair. `text` is "" in 96
// of the bundled palettes ("the terminal's default foreground"), so every
// Fg("text", ...) call site — dialog descriptions, list rows, login prompts,
// assistant prose — inherited whatever the user's terminal shows. Each palette
// records the body color it was designed against in terminal.foreground, and the
// loader adopts it, so those call sites draw a real color.
func TestPaletteForegroundIsAdopted(t *testing.T) {
	for _, name := range []string{"titanium", "dark-dracula", "light-solarized", "dark-github"} {
		theme := optionalPalette(t, name)
		if theme == nil {
			t.Fatalf("%s did not resolve", name)
		}
		if theme.Text == SGRFgReset || theme.Text == "" {
			t.Fatalf("%s: text is still the terminal default (%q)", name, theme.Text)
		}
		if theme.Text != theme.fgAnsi["text"] {
			t.Fatalf("%s: the Text field disagrees with the token table", name)
		}
		if theme.ProseFg() == "" {
			t.Fatalf("%s: prose fell back to the terminal default", name)
		}
		// The adopted color has to be a concrete color, not a re-registered default.
		if _, ok := theme.concreteColors["text"]; !ok {
			t.Fatalf("%s: the adopted text color is not in concreteColors", name)
		}
	}
}

// TestThemeNamingTextKeepsIt covers the other direction: a theme that writes a
// real color for `text` keeps exactly that color, and a theme with no terminal
// section keeps the terminal default it asked for.
func TestThemeNamingTextKeepsIt(t *testing.T) {
	for _, name := range []string{"light", "dark"} {
		// loadBuiltinThemeWithMode, not LoadBuiltinTheme: the latter resolves in
		// whatever mode the ambient terminal reports, so the assertions below would
		// mean different things on a truecolor terminal and on a CI runner.
		theme, err := loadBuiltinThemeWithMode(name, TerminalColorModeTrueColor)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if theme.Text == SGRFgReset {
			t.Fatalf("%s: text should be a real color", name)
		}
		if _, ok := theme.concreteColors["text"]; !ok {
			t.Fatalf("%s: text should resolve as a concrete color", name)
		}
	}

	// A theme assembled in code has no terminal section and no JSON source, so
	// adoption never ran and its own text color stands.
	bare := paletteHueTheme(t)
	if bare.source != nil {
		t.Fatalf("the fixture unexpectedly has JSON source")
	}
	if got := bare.Text; got != "\x1b[38;2;128;128;128m" {
		t.Fatalf("a theme without JSON source changed its text color to %q", got)
	}
}

// TestAssistantMessageTextWins covers the explicit token: a theme that names
// assistantMessageText paints prose in it rather than in `text`.
func TestAssistantMessageTextWins(t *testing.T) {
	theme := paletteHueTheme(t)
	theme.fgAnsi["text"] = "\x1b[38;2;99;99;99m"
	theme.fgAnsi[string(ThemeColorAssistantMessageText)] = "\x1b[38;2;1;2;3m"
	if got := theme.ProseFg(); got != "\x1b[38;2;1;2;3m" {
		t.Fatalf("assistantMessageText did not win over text: %q", got)
	}
}

// TestSystemThemeCoversEveryToken covers the generated theme against the whole
// token table. system_theme.go builds its colors from a family list, so a token
// added to the schema but not to that list resolves to nothing and panics the
// first component that draws it. The background variants are covered too: a
// terminal that reports a background and one that reports none take different
// paths (GenerateSystemThemeColors' palette tiers, systemIndexedColors), and the
// pending state renders grayscale before any color arrives.
func TestSystemThemeCoversEveryToken(t *testing.T) {
	background := &RgbColor{R: 26, G: 27, B: 38}
	foreground := &RgbColor{R: 192, G: 202, B: 245}
	palette := make([]RgbColor, ANSISlotCount)
	for i := range palette {
		palette[i] = RgbColor{R: float64(20 + i*13), G: float64(30 + i*11), B: float64(40 + i*9)}
	}
	appearance := TerminalTheme("dark")

	for _, tc := range []struct {
		name   string
		input  SystemThemeInput
		dimmed bool
	}{
		{"background and palette", SystemThemeInput{Foreground: foreground, Background: background, Palette: palette, AppearanceHint: appearance}, true},
		{"background only", SystemThemeInput{Foreground: foreground, Background: background, AppearanceHint: appearance}, true},
		{"nothing reported", SystemThemeInput{AppearanceHint: appearance}, false},
		{"grayscale pending", SystemThemeInput{Foreground: foreground, Background: background, Palette: palette, AppearanceHint: appearance, Saturation: new(0.0)}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			generated := GenerateSystemThemeColors(tc.input)
			foregroundValues, backgroundValues := splitThemeTokenValues(generated.Colors)
			theme, err := NewTheme(foregroundValues, backgroundValues, TerminalColorModeTrueColor,
				ThemeOptions{Name: SystemThemeName, Appearance: generated.Appearance, Dim: generated.Dim})
			if err != nil {
				t.Fatalf("build theme: %v", err)
			}
			for _, token := range ThemeColorTokens() {
				// Fg and Bg panic with "Unknown theme color" for a token the theme never
				// resolved, which is exactly the failure this guards.
				if themeBackgroundTokens[token.Name] {
					if ansi := theme.GetBgAnsi(token.Name); ansi == "" {
						t.Fatalf("%s: no background escape", token.Name)
					}
					continue
				}
				if ansi := theme.GetFgAnsi(token.Name); ansi == "" {
					t.Fatalf("%s: no foreground escape", token.Name)
				}
			}
		})
	}
}
