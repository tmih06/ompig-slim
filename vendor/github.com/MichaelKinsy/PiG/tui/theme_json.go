package tui

// theme_json.go ports upstream modes/interactive/theme/theme-schema.ts: the
// schema check for user-authored theme files. Pi validates with a compiled
// TypeBox schema; this walks the same schema in the same order and reports the
// same errors, including TypeBox's eight-error cap.

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
)

// themeJSONMaxErrors is TypeBox's default maxErrors setting.
const themeJSONMaxErrors = 8

type themeColorToken struct {
	name        string
	optional    bool
	description string
}

// themeColorTokens lists ThemeJsonSchema.colors in schema order (theme-tokens.ts THEME_TOKENS); optional tokens may be omitted.
var themeColorTokens = []themeColorToken{
	{"accent", false, "Primary accent color (logo, selected items, cursor)"},
	{"border", false, "Normal borders"},
	{"borderAccent", false, "Highlighted borders"},
	{"borderMuted", false, "Subtle borders"},
	{"success", false, "Success states"},
	{"error", false, "Error states"},
	{"warning", false, "Warning states"},
	{"muted", false, "Secondary/dimmed text"},
	{"dim", false, "Very dimmed text (more subtle than muted)"},
	{"text", false, "Default text color (usually empty string)"},
	{"thinkingText", false, "Thinking block text color"},
	{"scrollbarTrack", true, "Fullscreen scrollbar track foreground (falls back to muted when omitted)"},
	{"scrollbarThumb", true, "Fullscreen scrollbar thumb foreground (falls back to text when omitted)"},
	{"selectedBg", false, "Selected item background"},
	{"searchMatchBg", true, "Transcript search match background and current-match text (falls back to selectedBg when omitted)"},
	{"searchMatchText", true, "Transcript search match text and current-match background (falls back to text when omitted)"},
	{"userMessageBg", false, "User message background"},
	{"userMessageText", false, "User message text color"},
	{"customMessageBg", false, "Custom message background (hook-injected messages)"},
	{"customMessageText", false, "Custom message text color"},
	{"customMessageLabel", false, "Custom message type label color"},
	{"toolPendingBg", false, "Tool execution box (pending state)"},
	{"toolSuccessBg", false, "Tool execution box (success state)"},
	{"toolErrorBg", false, "Tool execution box (error state)"},
	{"toolTitle", false, "Tool execution box title color"},
	{"toolOutput", false, "Tool execution box output text color"},
	{"mdHeading", false, "Markdown heading text"},
	{"mdLink", false, "Markdown link text"},
	{"mdLinkUrl", false, "Markdown link URL"},
	{"mdCode", false, "Markdown inline code"},
	{"mdCodeBlock", false, "Markdown code block content"},
	{"mdCodeBlockBorder", false, "Markdown code block fences"},
	{"mdQuote", false, "Markdown blockquote text"},
	{"mdQuoteBorder", false, "Markdown blockquote border"},
	{"mdHr", false, "Markdown horizontal rule"},
	{"mdListBullet", false, "Markdown list bullets/numbers"},
	{"toolDiffAdded", false, "Added lines in tool diffs"},
	{"toolDiffRemoved", false, "Removed lines in tool diffs"},
	{"toolDiffContext", false, "Context lines in tool diffs"},
	{"syntaxComment", false, "Syntax highlighting: comments"},
	{"syntaxKeyword", false, "Syntax highlighting: keywords"},
	{"syntaxFunction", false, "Syntax highlighting: function names"},
	{"syntaxVariable", false, "Syntax highlighting: variable names"},
	{"syntaxString", false, "Syntax highlighting: string literals"},
	{"syntaxNumber", false, "Syntax highlighting: number literals"},
	{"syntaxType", false, "Syntax highlighting: type names"},
	{"syntaxOperator", false, "Syntax highlighting: operators"},
	{"syntaxPunctuation", false, "Syntax highlighting: punctuation"},
	{"thinkingOff", false, "Thinking level border: off"},
	{"thinkingMinimal", false, "Thinking level border: minimal"},
	{"thinkingLow", false, "Thinking level border: low"},
	{"thinkingMedium", false, "Thinking level border: medium"},
	{"thinkingHigh", false, "Thinking level border: high"},
	{"thinkingXhigh", false, "Thinking level border: xhigh"},
	{"thinkingMax", true, "Thinking level border: max (falls back to thinkingXhigh when omitted)"},
	{"bashMode", false, "Editor border color in bash mode"},
	{"pythonMode", true, "Editor border color in python mode (falls back to bashMode when omitted)"},

	// Status line segments. Upstream's schema has these tokens in the same object;
	// they are optional here so a theme written before the status line existed
	// still loads, and themeColorFallbacks gives each one a token to borrow.
	{"statusLineSep", true, "Status line separators (falls back to dim when omitted)"},
	{"statusLineModel", true, "Status line model name (falls back to accent when omitted)"},
	{"statusLinePath", true, "Status line working directory (falls back to border when omitted)"},
	{"statusLineGitClean", true, "Status line git branch, clean tree (falls back to success when omitted)"},
	{"statusLineGitDirty", true, "Status line git branch, dirty tree (falls back to warning when omitted)"},
	{"statusLineContext", true, "Status line context window (falls back to muted when omitted)"},
	{"statusLineSpend", true, "Status line token counters (falls back to accent when omitted)"},
	{"statusLineStaged", true, "Status line staged-file counter (falls back to success when omitted)"},
	{"statusLineDirty", true, "Status line modified-file counter (falls back to warning when omitted)"},
	{"statusLineUntracked", true, "Status line untracked-file counter (falls back to error when omitted)"},
	{"statusLineOutput", true, "Status line output-token counter (falls back to muted when omitted)"},
	{"statusLineCost", true, "Status line spend (falls back to success when omitted)"},
	{"statusLineSubagents", true, "Status line subagent counter (falls back to accent when omitted)"},

	// The assistant reply's prose. Upstream has it as OPTIONAL_THEME_COLOR_RECORD's
	// only member, so it is optional here too; unset keeps the terminal default.
	{"assistantMessageText", true, "Assistant reply prose (unset keeps the terminal default foreground)"},
}

// ThemeColorToken is one token of the theme "colors" object: its description and whether a theme may omit it.
type ThemeColorToken struct {
	Name, Description string
	Optional          bool
}

// themeTokenKnown reports whether name is a token this engine renders. Themes are shared with OMP, which defines color tokens of
// its own (`statusLine*`, `pythonMode`); those validate and are then ignored (theme.ts splitThemeColors skips keys without a
// THEME_TOKENS descriptor).
func themeTokenKnown(name string) bool {
	return slices.ContainsFunc(themeColorTokens, func(token themeColorToken) bool { return token.name == name })
}

// ThemeColorTokens returns the tokens of ThemeJsonSchema.colors in schema order.
func ThemeColorTokens() []ThemeColorToken {
	out := make([]ThemeColorToken, len(themeColorTokens))
	for i, t := range themeColorTokens {
		out[i] = ThemeColorToken{Name: t.name, Description: t.description, Optional: t.optional}
	}
	return out
}

// themeExportTokens lists ThemeJsonSchema.export in schema order.
var themeExportTokens = []string{"pageBg", "cardBg", "infoBg"}

// themeTerminalTokens lists ThemeJsonSchema.terminal's color members in schema order; ansi is checked separately because it is an array.
var themeTerminalTokens = []string{"background", "foreground", "chrome", "widget"}

// themeSymbolPresets are ThemeJsonSchema.symbols.preset's allowed values.
var themeSymbolPresets = []string{"unicode", "nerd", "ascii"}

type themeSchemaError struct {
	path, keyword, message string
	missing                []string
}

type themeSchemaErrors struct{ list []themeSchemaError }

func (e *themeSchemaErrors) add(err themeSchemaError) {
	if len(e.list) < themeJSONMaxErrors {
		e.list = append(e.list, err)
	}
}

// ValidateThemeJSON validates one theme document (JSON text without a BOM)
// and returns an error naming the offending tokens, or nil. Mirrors upstream
// validateThemeJson (theme-schema.ts): a name containing "/" is rejected before
// the schema runs.
func ValidateThemeJSON(label string, data []byte) error {
	var root any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return err
	}
	if object, ok := root.(map[string]any); ok {
		if name, ok := object["name"].(string); ok && strings.Contains(name, "/") {
			return errors.New("Invalid theme name \"" + name + "\": theme names cannot contain \"/\" because it is reserved for automatic light/dark theme settings.")
		}
	}
	var errs themeSchemaErrors
	checkThemeDocument(&errs, root, data)
	if len(errs.list) > 0 {
		return errors.New(formatThemeSchemaErrors(label, errs.list))
	}
	return nil
}

// checkAdditionalProperties reports additionalProperties:false the way TypeBox does: one "schema is false" error per unknown
// property in enumeration order, then the object's own error.
func checkAdditionalProperties(errs *themeSchemaErrors, path string, keys []string, known func(string) bool) {
	extra := false
	for _, key := range jsPropertyOrder(keys) {
		if !known(key) {
			extra = true
			errs.add(themeSchemaError{path: path + "/" + key, message: "schema is false"})
		}
	}
	if extra {
		errs.add(themeSchemaError{path: path, message: "must not have additional properties"})
	}
}

func checkThemeDocument(errs *themeSchemaErrors, root any, data []byte) {
	object, ok := root.(map[string]any)
	if !ok {
		errs.add(themeSchemaError{path: "", message: "must be object"})
		return
	}
	checkRequired(errs, "", object, []string{"name", "colors"})
	checkAdditionalProperties(errs, "", rawObjectKeys(data), func(key string) bool {
		return slices.Contains([]string{"$schema", "name", "appearance", "vars", "colors", "export", "terminal", "symbols"}, key)
	})
	if value, present := object["$schema"]; present {
		checkString(errs, "/$schema", value)
	}
	if value, present := object["name"]; present {
		checkThemeName(errs, value)
	}
	if value, present := object["appearance"]; present {
		checkThemeAppearance(errs, value)
	}
	if value, present := object["vars"]; present {
		if vars, ok := value.(map[string]any); ok {
			for _, key := range jsPropertyOrder(themeObjectKeys(data, "vars")) {
				checkThemeColorValue(errs, "/vars/"+key, vars[key])
			}
		} else {
			errs.add(themeSchemaError{path: "/vars", message: "must be object"})
		}
	}
	if value, present := object["colors"]; present {
		checkThemeColors(errs, value, themeObjectKeys(data, "colors"))
	}
	// ThemeJsonSchema.terminal and .symbols describe the terminal a theme was made for rather than anything this engine draws,
	// but OMP writes them into every built-in palette and into custom themes saved from one. They are checked and then ignored:
	// rejecting them made every OMP palette unreadable here, and silently dropping unknown keys would hide a real typo.
	if value, present := object["terminal"]; present {
		checkThemeTerminal(errs, value, themeObjectKeys(data, "terminal"))
	}
	if value, present := object["symbols"]; present {
		checkThemeSymbols(errs, value)
	}
	if value, present := object["export"]; present {
		exports, ok := value.(map[string]any)
		if !ok {
			errs.add(themeSchemaError{path: "/export", message: "must be object"})
			return
		}
		checkAdditionalProperties(errs, "/export", themeObjectKeys(data, "export"), func(key string) bool { return slices.Contains(themeExportTokens, key) })
		for _, token := range themeExportTokens {
			if value, present := exports[token]; present {
				checkThemeColorValue(errs, "/export/"+token, value)
			}
		}
	}
}

// checkThemeAppearance checks Optional(Union([Literal("dark"), Literal("light")])) the way TypeBox reports it: each literal's type and constant failures, then the union failure.
func checkThemeAppearance(errs *themeSchemaErrors, value any) {
	if appearance, ok := value.(string); ok && (appearance == "dark" || appearance == "light") {
		return
	}
	_, isString := value.(string)
	for range 2 {
		if !isString {
			errs.add(themeSchemaError{path: "/appearance", message: "must be string"})
		}
		errs.add(themeSchemaError{path: "/appearance", message: "must be equal to constant"})
	}
	errs.add(themeSchemaError{path: "/appearance", message: "must match a schema in anyOf"})
}

// checkThemeName checks String({pattern: "^[^/]+$"}): the string type first, then the pattern (a JavaScript [^/] matches a newline).
func checkThemeName(errs *themeSchemaErrors, value any) {
	name, ok := value.(string)
	if !ok {
		errs.add(themeSchemaError{path: "/name", message: "must be string"})
		return
	}
	if name == "" || strings.Contains(name, "/") {
		errs.add(themeSchemaError{path: "/name", message: `must match pattern "^[^/]+$"`})
	}
}

func checkThemeColors(errs *themeSchemaErrors, value any, keys []string) {
	colors, ok := value.(map[string]any)
	if !ok {
		errs.add(themeSchemaError{path: "/colors", message: "must be object"})
		return
	}
	var required []string
	for _, token := range themeColorTokens {
		if !token.optional {
			required = append(required, token.name)
		}
	}
	checkRequired(errs, "/colors", colors, required)
	for _, token := range themeColorTokens {
		if value, present := colors[token.name]; present {
			checkThemeColorValue(errs, "/colors/"+token.name, value)
		}
	}
	// Themes are shared with OMP through the same themes directory, and OMP defines color tokens this engine does not render
	// (`statusLine*`, `pythonMode`). Extra keys are validated as color values and then ignored, so one theme file works in both
	// tools; rejecting them made every OMP theme unusable here and told the user to edit files OMP still needs
	// (theme-schema.ts additionalProperties: colorValue(...)). A non-color extra value is still rejected, so a typo does not
	// slip through.
	for _, key := range jsPropertyOrder(keys) {
		if !themeTokenKnown(key) {
			checkThemeColorValue(errs, "/colors/"+key, colors[key])
		}
	}
}

// checkThemeTerminal checks ThemeJsonSchema.terminal: five color members, with ansi
// being exactly sixteen color values when it is present.
func checkThemeTerminal(errs *themeSchemaErrors, value any, keys []string) {
	terminal, ok := value.(map[string]any)
	if !ok {
		errs.add(themeSchemaError{path: "/terminal", message: "must be object"})
		return
	}
	checkAdditionalProperties(errs, "/terminal", keys, func(key string) bool {
		return slices.Contains(append(slices.Clone(themeTerminalTokens), "ansi"), key)
	})
	for _, token := range themeTerminalTokens {
		if member, present := terminal[token]; present {
			checkThemeColorValue(errs, "/terminal/"+token, member)
		}
	}
	if member, present := terminal["ansi"]; present {
		entries, ok := member.([]any)
		if !ok {
			errs.add(themeSchemaError{path: "/terminal/ansi", message: "must be array"})
			return
		}
		if len(entries) != ANSISlotCount {
			errs.add(themeSchemaError{path: "/terminal/ansi", message: "must not have fewer than 16 items"})
			errs.add(themeSchemaError{path: "/terminal/ansi", message: "must not have more than 16 items"})
		}
		for index, entry := range entries {
			checkThemeColorValue(errs, "/terminal/ansi/"+strconv.Itoa(index), entry)
		}
	}
}

// checkThemeSymbols checks ThemeJsonSchema.symbols: an optional preset name, a
// string map of glyph overrides, and optional spinner frames. Override keys are
// reported in sorted order because this check reads the decoded object, not the
// document, and a stable order keeps the error text diffable.
func checkThemeSymbols(errs *themeSchemaErrors, value any) {
	symbols, ok := value.(map[string]any)
	if !ok {
		errs.add(themeSchemaError{path: "/symbols", message: "must be object"})
		return
	}
	checkAdditionalProperties(errs, "/symbols", sortedObjectKeys(symbols), func(key string) bool {
		return slices.Contains([]string{"preset", "overrides", "spinnerFrames"}, key)
	})
	if preset, present := symbols["preset"]; present {
		name, isString := preset.(string)
		if !isString {
			errs.add(themeSchemaError{path: "/symbols/preset", message: "must be string"})
		} else if !slices.Contains(themeSymbolPresets, name) {
			errs.add(themeSchemaError{path: "/symbols/preset", message: "must be equal to one of the allowed values"})
		}
	}
	if overrides, present := symbols["overrides"]; present {
		entries, isObject := overrides.(map[string]any)
		if !isObject {
			errs.add(themeSchemaError{path: "/symbols/overrides", message: "must be object"})
			return
		}
		for _, key := range sortedObjectKeys(entries) {
			if _, isString := entries[key].(string); !isString {
				errs.add(themeSchemaError{path: "/symbols/overrides/" + key, message: "must be string"})
			}
		}
	}
	if frames, present := symbols["spinnerFrames"]; present {
		checkThemeSpinnerFrames(errs, "/symbols/spinnerFrames", frames)
	}
}

// checkThemeSpinnerFrames checks symbols.spinnerFrames: one non-empty string array
// for both spinner kinds, or an object naming either kind.
func checkThemeSpinnerFrames(errs *themeSchemaErrors, path string, value any) {
	if entries, ok := value.([]any); ok {
		checkThemeFrameArray(errs, path, entries)
		return
	}
	framing, ok := value.(map[string]any)
	if !ok {
		errs.add(themeSchemaError{path: path, message: "must match a schema in anyOf"})
		return
	}
	checkAdditionalProperties(errs, path, sortedObjectKeys(framing), func(key string) bool {
		return key == "status" || key == "activity"
	})
	named := false
	for _, key := range []string{"status", "activity"} {
		member, present := framing[key]
		if !present {
			continue
		}
		named = true
		if entries, ok := member.([]any); ok {
			checkThemeFrameArray(errs, path+"/"+key, entries)
		} else {
			errs.add(themeSchemaError{path: path + "/" + key, message: "must be array"})
		}
	}
	if !named {
		errs.add(themeSchemaError{path: path, message: "must have required properties status, activity"})
	}
}

// checkThemeFrameArray requires a spinner frame list to hold at least one non-empty string.
func checkThemeFrameArray(errs *themeSchemaErrors, path string, entries []any) {
	if len(entries) == 0 {
		errs.add(themeSchemaError{path: path, message: "must not have fewer than 1 items"})
	}
	for index, entry := range entries {
		frame, isString := entry.(string)
		if !isString {
			errs.add(themeSchemaError{path: path + "/" + strconv.Itoa(index), message: "must be string"})
		} else if frame == "" {
			errs.add(themeSchemaError{path: path + "/" + strconv.Itoa(index), message: "must not have fewer than 1 characters"})
		}
	}
}

// sortedObjectKeys returns object's keys in sorted order, the deterministic
// stand-in for insertion order where the raw document is unavailable.
func sortedObjectKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func checkRequired(errs *themeSchemaErrors, path string, object map[string]any, required []string) {
	var missing []string
	for _, name := range required {
		if _, present := object[name]; !present {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		errs.add(themeSchemaError{path: path, keyword: "required", message: "must have required properties " + strings.Join(missing, ", "), missing: missing})
	}
}

func checkString(errs *themeSchemaErrors, path string, value any) {
	if _, ok := value.(string); !ok {
		errs.add(themeSchemaError{path: path, message: "must be string"})
	}
}

// checkThemeColorValue checks Union([String, Integer{0..255}]) the way TypeBox
// reports it: the string branch, the integer branch's type and bound
// failures, then the union failure.
func checkThemeColorValue(errs *themeSchemaErrors, path string, value any) {
	if _, ok := value.(string); ok {
		return
	}
	number, finite := themeFiniteNumber(value)
	integer := finite && number == math.Trunc(number)
	if integer && number >= 0 && number <= 255 {
		return
	}
	errs.add(themeSchemaError{path: path, message: "must be string"})
	if !integer {
		errs.add(themeSchemaError{path: path, message: "must be integer"})
	}
	if finite && number < 0 {
		errs.add(themeSchemaError{path: path, message: "must be >= 0"})
	}
	if finite && number > 255 {
		errs.add(themeSchemaError{path: path, message: "must be <= 255"})
	}
	errs.add(themeSchemaError{path: path, message: "must match a schema in anyOf"})
}

func themeFiniteNumber(value any) (float64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(string(number), 64)
	if err != nil || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

func formatThemeSchemaErrors(label string, list []themeSchemaError) string {
	missing := map[string]bool{}
	var other []string
	for _, err := range list {
		if err.keyword == "required" && err.path == "/colors" {
			for _, name := range err.missing {
				missing[name] = true
			}
			continue
		}
		path := err.path
		if path == "" {
			path = "/"
		}
		other = append(other, "  - "+path+": "+err.message)
	}
	var b strings.Builder
	b.WriteString("Invalid theme \"" + label + "\":\n")
	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for name := range missing {
			names = append(names, "  - "+name)
		}
		slices.Sort(names)
		b.WriteString("\nMissing required color tokens:\n")
		b.WriteString(strings.Join(names, "\n"))
		b.WriteString("\n\nPlease add these colors to your theme's \"colors\" object.")
		b.WriteString("\nSee the built-in themes (dark.json, light.json) for reference values.")
	}
	if len(other) > 0 {
		b.WriteString("\n\nOther errors:\n")
		b.WriteString(strings.Join(other, "\n"))
	}
	return b.String()
}

// themeObjectKeys returns the keys of the top-level object member named field
// in document order, keeping the first position of a duplicated key as a
// JavaScript object does.
func themeObjectKeys(data []byte, field string) []string {
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		return nil
	}
	return rawObjectKeys(root[field])
}

// rawObjectKeys is themeObjectKeys for one JSON object value.
func rawObjectKeys(raw []byte) []string {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil
	}
	var keys []string
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return keys
		}
		key, _ := token.(string)
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
		var skip json.RawMessage
		if decoder.Decode(&skip) != nil {
			return keys
		}
	}
	return keys
}

// jsPropertyOrder orders keys as JavaScript enumerates an object's own
// properties: array-index keys ascending, then the rest in insertion order.
func jsPropertyOrder(keys []string) []string {
	var indexes []uint64
	var names []string
	for _, key := range keys {
		if n, err := strconv.ParseUint(key, 10, 32); err == nil && n < math.MaxUint32 && strconv.FormatUint(n, 10) == key {
			indexes = append(indexes, n)
			continue
		}
		names = append(names, key)
	}
	slices.Sort(indexes)
	ordered := make([]string, 0, len(keys))
	for _, n := range indexes {
		ordered = append(ordered, strconv.FormatUint(n, 10))
	}
	return append(ordered, names...)
}
