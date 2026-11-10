package tui

// Ports packages/coding-agent/src/modes/interactive/theme/theme.ts (loadTheme, resolveColor).

// theme_loader.go: JSON theme file loading.
//
// Loads themes from upstream-compatible JSON files (dark.json, light.json,
// or custom themes). The schema matches upstream theme/theme-schema.ts:
//
//	{
//	  "name": "dark",
//	  "vars": { "cyan": "#00d7ff", ... },
//	  "colors": { "accent": "cyan", "border": "blue", ... },
//	  "export": { "pageBg": "#18181e", ... }
//	}
//
// A color value is a hex string, "", a 256-color index, or a variable name
// resolved recursively through "vars" (theme.ts resolveVarRefs).
//
// Upstream reference: theme.ts:1-200 (loadTheme, resolveColor).

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/internal/jsonparse"
	"github.com/MichaelKinsy/PiG/internal/lazyregexp"
	"github.com/MichaelKinsy/PiG/internal/text"
)

//go:embed theme_dark.json theme_light.json theme_palettes.json
var builtinThemes embed.FS

// paletteDocuments once decodes the bundled palette bundle (theme_palettes.json):
// oh-my-pi's built-in themes, keyed by name, still as their JSON documents.
//
// The bundle is copied from oh-my-pi's packages/tui/src/theme/defaults.json
// (MIT, see NOTICE) so a user gets the same palettes in both tools rather than a
// single hand-tuned pair. Only the documents are decoded up front; a palette is
// resolved into ANSI sequences when it is selected, because resolving all 100
// costs about 35ms of the first frame's startup budget and a run uses one.
var paletteDocuments = sync.OnceValue(func() map[string]json.RawMessage {
	data, err := builtinThemes.ReadFile("theme_palettes.json")
	if err != nil {
		return nil
	}
	var documents map[string]json.RawMessage
	if err := json.Unmarshal(data, &documents); err != nil {
		return nil
	}
	return documents
})

// paletteResolved caches each bundled palette after its first use. Reading a
// palette's document in one entry skips re-parsing the rest of the bundle.
var paletteResolved sync.Map // name -> *Theme

// paletteTheme returns the bundled palette with the name, resolved on first use.
// A palette that fails to parse resolves to nil and stays nil: one broken entry
// must not take the theme picker down with it, and retrying it on every render
// would.
func paletteTheme(name string) *Theme {
	if cached, ok := paletteResolved.Load(name); ok {
		return cached.(*Theme)
	}
	raw, ok := paletteDocuments()[name]
	if !ok {
		return nil
	}
	var theme *Theme
	var tj ThemeJSON
	if err := json.Unmarshal(raw, &tj); err == nil && tj.Name != "" {
		if err := extractPaletteColorKeyOrder(raw, &tj); err == nil {
			if resolved, resolveErr := resolveThemeWithMode(&tj, TerminalColorModeTrueColor); resolveErr == nil {
				theme = resolved
			}
		}
	}
	paletteResolved.Store(name, theme)
	return theme
}

// extractPaletteColorKeyOrder fills tj.colorKeys from a palette document's raw
// bytes, so a bundled palette keeps its own "colors" order as a file-loaded one
// does.
func extractPaletteColorKeyOrder(raw []byte, tj *ThemeJSON) error {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return err
	}
	colors, ok := document["colors"]
	if !ok {
		return errors.New("palette has no colors")
	}
	tj.colorKeys = rawObjectKeys(colors)
	return nil
}

// BuiltinPaletteNames returns the names of the bundled palettes in sorted order.
// They are themes a user can select, not the dark/light pair this engine owns.
// A name is listed whether or not its palette parses: the registry resolves it
// on selection, and a palette that fails then is reported as a load failure
// rather than silently missing from the picker.
func BuiltinPaletteNames() []string {
	documents := paletteDocuments()
	names := make([]string, 0, len(documents))
	for name := range documents {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// LoadBuiltinTheme returns a built-in theme by name ("dark" or "light") in the terminal's color mode.
func LoadBuiltinTheme(name string) (*Theme, error) {
	return loadBuiltinThemeWithMode(name, GetTerminalColorMode())
}

// loadBuiltinThemeWithMode resolves a built-in theme in mode. The package-level built-ins use truecolor so that package initialization does not detect terminal capabilities, which can run a tmux probe; activation converts them to the terminal's mode (storeActiveTheme), as upstream creates a theme when it is set.
func loadBuiltinThemeWithMode(name string, mode TerminalColorMode) (*Theme, error) {
	filename := "theme_" + name + ".json"
	data, err := builtinThemes.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("builtin theme %q: %w", name, err)
	}
	var tj ThemeJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return nil, fmt.Errorf("parse builtin theme %q: %w", name, err)
	}
	tj.colorKeys = extractColorKeyOrder(data)
	return resolveThemeWithMode(&tj, mode)
}

// ThemeJSON is the upstream-compatible theme file schema.
// Mirrors upstream theme-schema.json.
type ThemeJSON struct {
	Name string `json:"name"`
	// Appearance is the background the theme is designed for, "dark" or "light"; empty means it is detected from the theme's colors.
	Appearance TerminalTheme              `json:"appearance,omitempty"`
	Vars       map[string]ThemeColorValue `json:"vars"`
	Colors     map[string]ThemeColorValue `json:"colors"`
	Export     map[string]ThemeColorValue `json:"export,omitempty"`
	// Terminal describes the terminal the theme was made for, as ThemeJsonSchema
	// declares it. This engine never paints the terminal, so only Foreground (the
	// body color a palette designed against a terminal default) and ANSI (the palette
	// colorful Markdown draws its hues from) are read.
	Terminal *ThemeTerminalJSON `json:"terminal,omitempty"`
	// colorKeys preserves the insertion order of keys in the "colors"
	// JSON object. Populated by extractColorKeyOrder after decoding.
	colorKeys []string
}

// ThemeTerminalJSON is ThemeJsonSchema.terminal: the colors of the terminal the theme
// was written for, plus the ANSI palette 0-15.
type ThemeTerminalJSON struct {
	Background ThemeColorValue   `json:"background,omitempty"`
	Foreground ThemeColorValue   `json:"foreground,omitempty"`
	Chrome     ThemeColorValue   `json:"chrome,omitempty"`
	Widget     ThemeColorValue   `json:"widget,omitempty"`
	ANSI       []ThemeColorValue `json:"ansi,omitempty"`
}

// LoadThemeFile reads a theme JSON file and returns the resolved Theme in the terminal's color mode, reporting invalid JSON with ECMAScript SyntaxError text.
func LoadThemeFile(path string) (*Theme, error) {
	return LoadThemeFromPath(path, GetTerminalColorMode())
}

// LoadThemeFromPath reads a theme JSON file and returns the resolved Theme in mode (theme.ts loadThemeFromPath).
func LoadThemeFromPath(path string, mode TerminalColorMode) (*Theme, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read theme %s: %w", path, err)
	}
	data := text.StripBomBytes(raw)
	if err := jsonparse.Validate(data); err != nil {
		return nil, fmt.Errorf("Failed to parse theme %s: SyntaxError: %w", path, err)
	}
	// Pi validates every user-authored theme before use (theme-schema.ts).
	if err := ValidateThemeJSON(path, data); err != nil {
		return nil, err
	}
	var tj ThemeJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return nil, fmt.Errorf("parse theme %s: %w", path, err)
	}
	tj.colorKeys = extractColorKeyOrder(data)
	theme, err := resolveThemeWithMode(&tj, mode)
	if err != nil {
		return nil, err
	}
	// theme.ts loadThemeFromPath: createTheme(themeJson, mode, themePath) records where the theme was loaded from.
	theme.SourcePath = path
	return theme, nil
}

// ThemeColorValue mirrors theme-schema.ts ColorValue: a hex color, variable
// reference or empty string (Text), or a 256-color palette index (Index, when
// IsIndex is set).
type ThemeColorValue struct {
	Text    string
	Index   int
	IsIndex bool
	isSet   bool
}

// UnmarshalJSON accepts a JSON string or an integral JSON number. The 0..255
// range is enforced by ValidateThemeJSON for user-authored themes.
func (v *ThemeColorValue) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*v = ThemeColorValue{Text: text, isSet: true}
		return nil
	}
	var number float64
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("theme color must be a string or 256-color index: %s", data)
	}
	if number != math.Trunc(number) {
		return fmt.Errorf("theme color index must be an integer: %s", data)
	}
	//portlint:allow numbers an index beyond int range converts to an extreme value on every architecture, which NewIndexedColor rejects as theme-schema.ts colorValue Integer 0..255 does
	*v = ThemeColorValue{Index: int(number), IsIndex: true, isSet: true}
	return nil
}

// MarshalJSON writes the value as the JSON string or number it was read from (theme-schema.ts ColorValue is `string | number`); an unset value writes the empty string.
func (v ThemeColorValue) MarshalJSON() ([]byte, error) {
	if v.IsIndex {
		return json.Marshal(v.Index)
	}
	return json.Marshal(v.Text)
}

// exportColor mirrors getThemeExportColors' per-value conversion: an index becomes hex and an empty value stays unset. Export colors end up in CSS, which understands hex and oklch() directly but not okhsl(), so okhsl() becomes hex (theme.ts:928-940).
func (v ThemeColorValue) exportColor() string {
	if !v.isSet {
		return ""
	}
	if v.IsIndex {
		return ColorToHex(IndexedColor{Index: v.Index})
	}
	if v.Text == "" {
		return ""
	}
	if themeOkhslPrefix.MatchString(v.Text) {
		if color, err := ParseColor(v.Text); err == nil {
			return ColorToHex(color)
		}
	}
	return v.Text
}

var (
	themeOkhslPrefix      = lazyregexp.New(`(?i)^okhsl\(`)
	themeOklchOkhslPrefix = lazyregexp.New(`(?i)^ok(?:lch|hsl)\(`)
)

// resolveVarRefs mirrors theme.ts resolveVarRefs: indexes, "", "#..." and
// oklch()/okhsl() literals are final; any other string names a variable,
// resolved recursively.
func resolveVarRefs(value ThemeColorValue, vars map[string]ThemeColorValue, visited map[string]bool) (ThemeColorValue, error) {
	if value.IsIndex || value.Text == "" || strings.HasPrefix(value.Text, "#") || themeOklchOkhslPrefix.MatchString(value.Text) {
		return value, nil
	}
	if visited[value.Text] {
		return ThemeColorValue{}, fmt.Errorf("Circular variable reference detected: %s", value.Text)
	}
	next, ok := vars[value.Text]
	if !ok {
		return ThemeColorValue{}, fmt.Errorf("Variable reference not found: %s", value.Text)
	}
	if visited == nil {
		visited = map[string]bool{}
	}
	visited[value.Text] = true
	return resolveVarRefs(next, vars, visited)
}

// LoadThemeDir scans a directory for .json theme files and returns a map
// of name → *Theme. Mirrors upstream theme discovery.
func LoadThemeDir(dir string) (map[string]*Theme, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	themes := make(map[string]*Theme)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		// Skip schema files.
		if strings.Contains(e.Name(), "schema") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		t, err := LoadThemeFile(path)
		if err != nil {
			continue // skip broken themes
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		themes[name] = t
	}
	return themes, nil
}

// extractColorKeyOrder parses the raw JSON to extract the key ordering of
// the "colors" object. json.Decoder preserves token order, unlike
// json.Unmarshal into map[string]string which loses insertion order.
func extractColorKeyOrder(data []byte) []string {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	// Find the "colors" key at the top level.
	var keys []string
	depth := 0
	inColors := false
	for {
		t, err := dec.Token()
		if err != nil {
			break
		}
		switch v := t.(type) {
		case json.Delim:
			switch v {
			case '{':
				if inColors && depth == 1 {
					// We're inside the colors object at depth 2.
					// Read key-value pairs.
					for dec.More() {
						kt, err := dec.Token()
						if err != nil {
							return keys
						}
						if key, ok := kt.(string); ok {
							keys = append(keys, key)
						}
						// Skip the value.
						if _, err := dec.Token(); err != nil {
							return keys
						}
					}
					return keys
				}
				depth++
			case '}':
				depth--
				inColors = false
			case '[':
				// skip arrays
			case ']':
				// skip arrays
			}
		case string:
			if depth == 1 && v == "colors" {
				inColors = true
			} else {
				inColors = false
			}
		}
	}
	return keys
}

// themeColorFallbacks lists the optional tokens and the token each falls back to,
// in upstream withThemeColorFallbacks order. The pythonMode and statusLine* entries
// are this engine's additions: upstream's schema requires them, while a theme
// written before the status line has no reason to name them, and a footer segment
// whose token is missing has no color to draw with.
var themeColorFallbacks = []struct{ token, from string }{
	{"scrollbarTrack", "muted"},
	{"scrollbarThumb", "text"},
	{"thinkingMax", "thinkingXhigh"},
	{"searchMatchBg", "selectedBg"},
	{"searchMatchText", "text"},
	{"pythonMode", "bashMode"},
	{"statusLineSep", "dim"},
	{"statusLineModel", "accent"},
	{"statusLinePath", "border"},
	{"statusLineGitClean", "success"},
	{"statusLineGitDirty", "warning"},
	{"statusLineContext", "muted"},
	{"statusLineSpend", "accent"},
	{"statusLineStaged", "success"},
	{"statusLineDirty", "warning"},
	{"statusLineUntracked", "error"},
	{"statusLineOutput", "muted"},
	{"statusLineCost", "success"},
	{"statusLineSubagents", "accent"},
	// assistantMessageText is optional and has no token to borrow: an omitted one
	// is the terminal's default foreground, which is what every theme rendered
	// prose in before the token existed. "text" is not a substitute, because most
	// palettes set text to the terminal default as well.
	{"assistantMessageText", ""},
}

// themeBackgroundTokens are the tokens drawn as backgrounds (theme.ts BACKGROUND_TOKENS).
var themeBackgroundTokens = map[string]bool{
	"selectedBg": true, "searchMatchBg": true, "userMessageBg": true, "customMessageBg": true,
	"toolPendingBg": true, "toolSuccessBg": true, "toolErrorBg": true,
}

// resolveThemeWithMode mirrors theme.ts createTheme with an explicit color mode. The unmodified JSON is retained so the theme can be rebuilt in the other mode when truecolor support changes.
func resolveThemeWithMode(tj *ThemeJSON, mode TerminalColorMode) (*Theme, error) {
	source := cloneThemeJSON(tj)
	// Resolve in JSON order so the first failing reference is the one upstream reports.
	resolved := make(map[string]ThemeColorValue, len(tj.Colors))
	keys := slices.Clone(tj.colorKeys)
	for _, k := range tj.colorKeys {
		value, ok := tj.Colors[k]
		if !ok {
			continue
		}
		next, err := resolveVarRefs(value, tj.Vars, nil)
		if err != nil {
			return nil, err
		}
		resolved[k] = next
	}
	// Optional tokens use nullish fallbacks: only an omitted token falls back, while an explicitly configured value, including "", is retained. A
	// fallback naming no token resolves to the terminal default, the "no opinion" value for a token whose absence means "inherit".
	for _, fallback := range themeColorFallbacks {
		if _, present := tj.Colors[fallback.token]; present {
			continue
		}
		if fallback.from != "" {
			resolved[fallback.token] = resolved[fallback.from]
		} else {
			resolved[fallback.token] = ThemeColorValue{isSet: true}
		}
		keys = append(keys, fallback.token)
	}
	// Themes shared with OMP carry color tokens this engine does not render. They validate so one
	// file serves both tools, and are dropped here rather than indexed blindly, which would fail on their missing descriptor
	// (theme.ts splitThemeColors). The theme's own key order is only the tokens it renders, as source Theme's is.
	keys = slices.DeleteFunc(keys, func(token string) bool { return !themeTokenKnown(token) })
	tj.colorKeys = keys

	foregrounds := make([]ThemeTokenValue, 0, len(keys))
	backgrounds := make([]ThemeTokenValue, 0, len(keys))
	for _, token := range keys {
		entry := ThemeTokenValue{Token: token, Value: resolved[token]}
		if themeBackgroundTokens[token] {
			backgrounds = append(backgrounds, entry)
		} else {
			foregrounds = append(foregrounds, entry)
		}
	}
	theme, err := NewTheme(foregrounds, backgrounds, mode, ThemeOptions{Name: tj.Name, Appearance: tj.Appearance})
	if err != nil {
		return nil, err
	}
	theme.source = source
	theme.ansiFg = resolveThemeANSIPalette(tj, mode)
	// A palette that leaves `text` at the terminal default records the body color it
	// was designed against in terminal.foreground; adopting it here makes every
	// existing Fg("text", ...) call site (dialogs, descriptions, login prompts) draw
	// that color instead of inheriting the terminal's own foreground.
	theme.adoptPaletteForeground()
	exportColors := resolveThemeExportColors(tj)
	theme.ExportPageBg, theme.ExportCardBg, theme.ExportInfoBg = exportColors["pageBg"], exportColors["cardBg"], exportColors["infoBg"]
	return theme, nil
}

func cloneThemeJSON(tj *ThemeJSON) *ThemeJSON {
	clone := *tj
	clone.colorKeys = slices.Clone(tj.colorKeys)
	return &clone
}

// resolveThemeANSIPalette resolves the theme's "terminal.ansi" colors to the 16
// foreground escapes colorful Markdown paints from, or nil when the theme names
// no palette (a theme written for PiG, which has no terminal section). A slot
// whose value cannot be resolved, or that is the terminal default, stays "", and
// its role falls back to the terminal's own palette entry (see Theme.ANSIFg).
func resolveThemeANSIPalette(tj *ThemeJSON, mode TerminalColorMode) []string {
	if tj.Terminal == nil || len(tj.Terminal.ANSI) == 0 {
		return nil
	}
	palette := make([]string, ANSISlotCount)
	for slot := 0; slot < ANSISlotCount && slot < len(tj.Terminal.ANSI); slot++ {
		resolved, err := resolveVarRefs(tj.Terminal.ANSI[slot], tj.Vars, nil)
		if err != nil {
			continue
		}
		var color Color
		if resolved.IsIndex {
			color, err = NewIndexedColor(resolved.Index)
		} else if resolved.Text != "" {
			color, err = ParseColor(resolved.Text)
		} else {
			continue
		}
		if err == nil {
			palette[slot] = ForegroundAnsi(color, mode)
		}
	}
	return palette
}

// resolveThemeExportColors mirrors theme.ts getThemeExportColors: CSS colors
// for the export section, with indexes converted to hex and "" left unset.
// Any unresolvable reference leaves every export color unset.
func resolveThemeExportColors(tj *ThemeJSON) map[string]string {
	out := make(map[string]string, len(themeExportTokens))
	for _, name := range themeExportTokens {
		value, ok := tj.Export[name]
		if !ok {
			continue
		}
		resolved, err := resolveVarRefs(value, tj.Vars, nil)
		if err != nil {
			return nil
		}
		out[name] = resolved.exportColor()
	}
	return out
}
