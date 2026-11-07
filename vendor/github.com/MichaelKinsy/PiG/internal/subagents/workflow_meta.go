package subagents

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type WorkflowMeta struct {
	Name        string                                  `json:"name"`
	Description string                                  `json:"description"`
	WhenToUse   string                                  `json:"whenToUse,omitempty"`
	Phases      []struct{ Title, Detail, Model string } `json:"phases,omitempty"`
}

// A small literal parser, deliberately not an evaluator: metadata may contain
// JSON values, JS quoted strings, identifier keys, comments and trailing commas.
// Computed keys, spreads, references and calls never execute during discovery.
type literalParser struct {
	s string
	i int
}

func (p *literalParser) skip() {
	for p.i < len(p.s) {
		if unicode.IsSpace(rune(p.s[p.i])) {
			p.i++
			continue
		}
		if strings.HasPrefix(p.s[p.i:], "//") {
			if n := strings.IndexByte(p.s[p.i:], '\n'); n >= 0 {
				p.i += n + 1
				continue
			}
			p.i = len(p.s)
			return
		}
		if strings.HasPrefix(p.s[p.i:], "/*") {
			if n := strings.Index(p.s[p.i+2:], "*/"); n >= 0 {
				p.i += n + 4
				continue
			}
		}
		break
	}
}
func (p *literalParser) quoted() (string, error) {
	start := p.i
	q := p.s[p.i]
	p.i++
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		p.i++
		if c == q {
			return b.String(), nil
		}
		if c == '\n' || c == '\r' {
			return "", fmt.Errorf("newline in metadata string")
		}
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		if p.i >= len(p.s) {
			break
		}
		e := p.s[p.i]
		p.i++
		switch e {
		case '\'', '"', '\\', '/':
			b.WriteByte(e)
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'u':
			if p.i+4 > len(p.s) {
				return "", fmt.Errorf("invalid unicode escape")
			}
			raw := "\"\\u" + p.s[p.i:p.i+4] + "\""
			var decoded string
			if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
				return "", err
			}
			b.WriteString(decoded)
			p.i += 4
		default:
			return "", fmt.Errorf("invalid string escape")
		}
	}
	return "", fmt.Errorf("unclosed metadata string at %d", start)
}
func (p *literalParser) value(depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("metadata too deeply nested")
	}
	p.skip()
	if p.i >= len(p.s) {
		return nil, fmt.Errorf("unexpected end of metadata")
	}
	c := p.s[p.i]
	if c == '\'' || c == '"' {
		return p.quoted()
	}
	if c == '{' {
		p.i++
		out := map[string]any{}
		for {
			p.skip()
			if p.i < len(p.s) && p.s[p.i] == '}' {
				p.i++
				return out, nil
			}
			var key string
			var err error
			if p.i < len(p.s) && (p.s[p.i] == '\'' || p.s[p.i] == '"') {
				key, err = p.quoted()
			} else {
				start := p.i
				for p.i < len(p.s) && (unicode.IsLetter(rune(p.s[p.i])) || unicode.IsDigit(rune(p.s[p.i])) || p.s[p.i] == '_' || p.s[p.i] == '$') {
					p.i++
				}
				key = p.s[start:p.i]
				if key == "" {
					err = fmt.Errorf("meta must be a pure literal object")
				}
			}
			if err != nil {
				return nil, err
			}
			p.skip()
			if p.i >= len(p.s) || p.s[p.i] != ':' {
				return nil, fmt.Errorf("metadata key %s needs a colon", key)
			}
			p.i++
			v, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			out[key] = v
			p.skip()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.i < len(p.s) && p.s[p.i] == '}' {
				continue
			}
			return nil, fmt.Errorf("expected metadata comma or closing brace")
		}
	}
	if c == '[' {
		p.i++
		out := []any{}
		for {
			p.skip()
			if p.i < len(p.s) && p.s[p.i] == ']' {
				p.i++
				return out, nil
			}
			v, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			p.skip()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.i < len(p.s) && p.s[p.i] == ']' {
				continue
			}
			return nil, fmt.Errorf("expected array comma or closing bracket")
		}
	}
	start := p.i
	for p.i < len(p.s) && !strings.ContainsRune(" \t\r\n,]}/", rune(p.s[p.i])) {
		p.i++
	}
	word := p.s[start:p.i]
	switch word {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	if n, err := strconv.ParseFloat(word, 64); err == nil && json.Valid([]byte(word)) {
		return n, nil
	}
	return nil, fmt.Errorf("meta must contain only literal values, found %q", word)
}
func ExtractMeta(source string) (WorkflowMeta, string, error) {
	p := literalParser{s: strings.TrimPrefix(source, "\ufeff")}
	p.skip()
	start := p.i
	if !strings.HasPrefix(p.s[p.i:], "export const meta") {
		return WorkflowMeta{}, "", fmt.Errorf("workflow must begin with export const meta = { name, description }")
	}
	p.i += len("export const meta")
	p.skip()
	if p.i >= len(p.s) || p.s[p.i] != '=' {
		return WorkflowMeta{}, "", fmt.Errorf("expected meta assignment")
	}
	p.i++
	v, err := p.value(0)
	if err != nil {
		return WorkflowMeta{}, "", err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return WorkflowMeta{}, "", err
	}
	var meta WorkflowMeta
	if err = json.Unmarshal(b, &meta); err != nil {
		return meta, "", err
	}
	if strings.TrimSpace(meta.Name) == "" || strings.TrimSpace(meta.Description) == "" {
		return meta, "", fmt.Errorf("meta.name and meta.description are required")
	}
	return meta, p.s[:start] + strings.TrimPrefix(p.s[start:], "export "), nil
}
