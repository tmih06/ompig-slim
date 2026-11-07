package tui

import "testing"

func TestNativeThemeSurfacesRemainDistinctAndReadable(t *testing.T) {
	for _, appearance := range []TerminalTheme{"dark", "light"} {
		generated := GenerateSystemThemeColors(SystemThemeInput{AppearanceHint: appearance})
		backgrounds := map[string]bool{}
		indexedBackgrounds := map[string]bool{}
		for _, token := range []string{"selectedBg", "userMessageBg", "customMessageBg", "toolPendingBg", "toolSuccessBg", "toolErrorBg"} {
			value := generated.Colors[token].Text
			if value == "" || backgrounds[value] {
				t.Fatalf("%s %s has an absent or duplicate surface: %q", appearance, token, value)
			}
			backgrounds[value] = true
			bg, err := ParseColor(value)
			if err != nil {
				t.Fatal(err)
			}
			indexed := ColorToHex(IndexedColor{Index: rgbToAnsi256(ColorToRgb(bg))})
			if indexedBackgrounds[indexed] {
				t.Errorf("%s %s loses its distinct surface in 256 colors", appearance, token)
			}
			indexedBackgrounds[indexed] = true
			for _, foreground := range []string{"text", "muted", "toolOutput"} {
				fg, err := ParseColor(generated.Colors[foreground].Text)
				if err != nil {
					t.Fatal(err)
				}
				for _, mode := range []TerminalColorMode{TerminalColorModeTrueColor, TerminalColorMode256} {
					text, surface := ColorToRgb(fg), ColorToRgb(bg)
					if mode == TerminalColorMode256 {
						text = ColorToRgb(IndexedColor{Index: rgbToAnsi256(text)})
						surface = ColorToRgb(IndexedColor{Index: rgbToAnsi256(surface)})
					}
					first, second := relativeLuminance(text), relativeLuminance(surface)
					contrast := (max(first, second) + .05) / (min(first, second) + .05)
					if contrast < 4.5 {
						t.Errorf("%s %s: %s on %s has %.2f:1 contrast; need 4.5:1", appearance, mode, foreground, token, contrast)
					}
				}
			}
		}
	}
}

func TestNativeFallbackWaitsForColorDiscovery(t *testing.T) {
	pending := 0.0
	colors := GenerateSystemThemeColors(SystemThemeInput{AppearanceHint: "dark", Saturation: &pending})
	if colors.Colors["selectedBg"].Text != "" || colors.Colors["accent"].IsIndex {
		t.Fatal("pending discovery must keep its neutral first frame")
	}
	background := RgbColor{R: 18, G: 22, B: 30}
	adaptive := GenerateSystemThemeColors(SystemThemeInput{AppearanceHint: "dark", Background: &background})
	fallback := GenerateSystemThemeColors(SystemThemeInput{AppearanceHint: "dark"})
	if adaptive.Colors["toolPendingBg"].Text == fallback.Colors["toolPendingBg"].Text {
		t.Fatal("reported backgrounds must still use the adaptive contrast solver")
	}
}
