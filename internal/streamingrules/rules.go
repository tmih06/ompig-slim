package streamingrules

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
	"go.yaml.in/yaml/v3"
)

type stringsField []string

func (s *stringsField) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		*s = []string{node.Value}
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("expected a string or list of strings")
	}
	var values []string
	for _, child := range node.Content {
		if child.Tag != "!!str" {
			return fmt.Errorf("expected a list of strings")
		}
		values = append(values, child.Value)
	}
	*s = values
	return nil
}

type metadata struct {
	Enabled       *bool        `yaml:"enabled"`
	Description   string       `yaml:"description"`
	Condition     stringsField `yaml:"condition"`
	LegacyTrigger stringsField `yaml:"ttsr_trigger"`
	Scope         stringsField `yaml:"scope"`
	InterruptMode string       `yaml:"interruptMode"`
	ASTCondition  any          `yaml:"astCondition"`
	Question      any          `yaml:"question"`
	RepeatMode    string       `yaml:"repeatMode"`
	Globs         any          `yaml:"globs"`
	Agents        any          `yaml:"agents"`
}

type scope struct {
	tool string
	glob string
}

type rule struct {
	name      string
	file      string
	body      string
	summary   string
	patterns  []*regexp2.Regexp
	scopes    []scope
	interrupt bool
	builtin   bool
}

var scopePattern = regexp.MustCompile(`^tool:([\w-]+)(?:\(([^()]+)\))?$`)

func parseRule(name, file, source string, builtin bool) (*rule, bool, error) {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	if !strings.HasPrefix(source, "---\n") {
		return nil, false, nil
	}
	header, body, ok := strings.Cut(source[4:], "\n---")
	if !ok || (body != "" && !strings.HasPrefix(body, "\n")) {
		return nil, false, fmt.Errorf("unclosed frontmatter")
	}
	var meta metadata
	if err := yaml.Unmarshal([]byte(header), &meta); err != nil {
		return nil, false, fmt.Errorf("invalid frontmatter")
	}
	if meta.Enabled != nil && !*meta.Enabled {
		return nil, true, nil
	}
	if len(meta.Condition) == 0 {
		meta.Condition = meta.LegacyTrigger
	}
	if len(meta.Condition) == 0 && meta.ASTCondition == nil && meta.Question == nil {
		return nil, false, nil
	}
	if meta.ASTCondition != nil || meta.Question != nil || meta.Globs != nil || meta.Agents != nil || (meta.RepeatMode != "" && meta.RepeatMode != "once") {
		return nil, false, fmt.Errorf("unsupported streaming rule metadata")
	}
	r := &rule{name: name, file: file, body: strings.TrimSpace(body), builtin: builtin}
	if r.body == "" {
		return nil, false, fmt.Errorf("rule body is empty")
	}
	r.summary = summarizePattern(meta.Description, r.body)
	switch meta.InterruptMode {
	case "", "always", "tool-only":
		r.interrupt = true
	case "never":
	default:
		return nil, false, fmt.Errorf("interruptMode must be always, tool-only, or never")
	}
	for _, condition := range meta.Condition {
		if condition == "" {
			return nil, false, fmt.Errorf("condition is empty")
		}
		pattern, err := regexp2.Compile(condition, regexp2.ECMAScript)
		if err != nil {
			return nil, false, fmt.Errorf("invalid regex condition")
		}
		pattern.MatchTimeout = 50 * time.Millisecond
		r.patterns = append(r.patterns, pattern)
	}
	if len(meta.Scope) == 0 {
		meta.Scope = stringsField{"tool:edit", "tool:write"}
	}
	for _, group := range meta.Scope {
		for _, token := range strings.Split(group, ",") {
			token = strings.TrimSpace(token)
			if token == "tool" || token == "toolcall" {
				r.scopes = append(r.scopes, scope{})
				continue
			}
			parts := scopePattern.FindStringSubmatch(token)
			if parts == nil {
				return nil, false, fmt.Errorf("unsupported scope %q", token)
			}
			if _, err := path.Match(parts[2], ""); err != nil || strings.Contains(parts[2], "**") {
				return nil, false, fmt.Errorf("invalid path glob %q", parts[2])
			}
			r.scopes = append(r.scopes, scope{tool: parts[1], glob: parts[2]})
		}
	}
	return r, false, nil
}

type snapshot struct {
	tool string
	path string
	text string
}

func toolSnapshots(tool string, args map[string]any) []snapshot {
	file, _ := args["path"].(string)
	switch tool {
	case "write":
		content, ok := args["content"].(string)
		if ok {
			return []snapshot{{tool: tool, path: file, text: content}}
		}
	case "edit":
		var result []snapshot
		if edits, ok := args["edits"].([]any); ok {
			for _, value := range edits {
				if edit, ok := value.(map[string]any); ok {
					if text, ok := edit["newText"].(string); ok {
						result = append(result, snapshot{tool: tool, path: file, text: text})
					}
				}
			}
		} else if text, ok := args["newText"].(string); ok {
			result = append(result, snapshot{tool: tool, path: file, text: text})
		}
		return result
	case "codemode":
		return nil
	default:
		data, err := json.Marshal(args)
		if err == nil {
			return []snapshot{{tool: tool, path: file, text: string(data)}}
		}
	}
	return nil
}

func (r *rule) matches(s snapshot) bool {
	allowed := false
	for _, scope := range r.scopes {
		if scope.tool != "" && scope.tool != s.tool {
			continue
		}
		if scope.glob != "" {
			file := strings.ReplaceAll(s.path, `\`, "/")
			if file == "" {
				continue
			}
			if !strings.Contains(scope.glob, "/") {
				file = path.Base(file)
			}
			if matched, _ := path.Match(scope.glob, file); !matched {
				continue
			}
		}
		allowed = true
		break
	}
	if !allowed {
		return false
	}
	for _, pattern := range r.patterns {
		matched, err := pattern.MatchString(s.text)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func (r *rule) reminder(blocked bool) string {
	instruction := "A user-defined rule matched. Follow this rule."
	if r.builtin {
		instruction = "A built-in style rule matched. Check whether this is a real violation, including any exceptions, and fix it only if needed."
	}
	if blocked {
		instruction += " The tool did not run. Revise the call before trying again."
	}
	return fmt.Sprintf("<system-reminder reason=\"rule_violation\" rule=%q path=%q>\n%s\n\n%s\n</system-reminder>", r.name, r.file, instruction, r.body)
}
