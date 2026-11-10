package tui

import (
	"strings"
	"unicode"
)

// SkillToken identifies a whitespace-delimited invocation with byte offsets in the draft.
type SkillToken struct {
	Start, End int
	Name       string
}

// AllowsSkillTokens excludes command arguments and local execution, which consume literal text.
func AllowsSkillTokens(text string) bool {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "/skill:") {
		return true
	}
	if strings.HasPrefix(text, "/") || strings.HasPrefix(text, "!") {
		return false
	}
	if strings.HasPrefix(text, "$") && !strings.HasPrefix(text, "${") {
		rest := strings.TrimPrefix(strings.TrimPrefix(text, "$"), "$")
		if rest == "" || strings.IndexFunc(rest, unicode.IsSpace) == 0 {
			return false
		}
	}
	return true
}

// SkillTokens returns complete skill invocations in source order, including namespaced names.
// Unknown names remain tokens; callers decide whether they are registered skills.
func SkillTokens(text string) []SkillToken {
	var tokens []SkillToken
	for start := 0; start < len(text); {
		offset := strings.IndexFunc(text[start:], func(r rune) bool { return !unicode.IsSpace(r) })
		if offset < 0 {
			break
		}
		start += offset
		end := strings.IndexFunc(text[start:], unicode.IsSpace)
		if end < 0 {
			end = len(text)
		} else {
			end += start
		}
		token := text[start:end]
		if name, ok := strings.CutPrefix(token, "/skill:"); ok && name != "" && !strings.HasPrefix(name, "/") && !strings.HasSuffix(name, "/") && strings.Count(name, "/") <= 1 {
			tokens = append(tokens, SkillToken{Start: start, End: end, Name: name})
		}
		start = end
	}
	return tokens
}

// trailingSkillSlash finds a slash lookup at a whitespace boundary, never a path or URL.
func trailingSkillSlash(before string) (string, bool) {
	start := 0
	for i, r := range before {
		if unicode.IsSpace(r) {
			start = i + len(string(r))
		}
	}
	token := before[start:]
	return token, strings.HasPrefix(token, "/") && (!strings.Contains(token[1:], "/") || (strings.HasPrefix(token, "/skill:") && strings.Count(token, "/") <= 2))
}
