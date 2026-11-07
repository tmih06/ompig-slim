// Package subagents ports tintinweb/pi-subagents into the native session host.
// Reference: https://github.com/tintinweb/pi-subagents/tree/50460b5df6934c4237983a504808a04ee0f8a0a0
package subagents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Settings uses the extension's field names and project-over-global precedence.
// Enabled is the harness's built-in activation switch; it defaults to false.
type Settings struct {
	Enabled                 bool   `json:"enabled"`
	MaxConcurrent           int    `json:"maxConcurrent"`
	MaxConcurrentForeground int    `json:"maxConcurrentForeground"`
	DefaultMaxTurns         int    `json:"defaultMaxTurns"`
	GraceTurns              int    `json:"graceTurns"`
	MaxSubagentDepth        int    `json:"maxSubagentDepth"`
	DefaultJoinMode         string `json:"defaultJoinMode"`
	BackgroundByDefault     bool   `json:"backgroundByDefault"`
	SchedulingEnabled       bool   `json:"schedulingEnabled"`
	ScopeModels             bool   `json:"scopeModels"`
	StrictAgentFiles        bool   `json:"strictAgentFiles"`
	DisableDefaultAgents    bool   `json:"disableDefaultAgents"`
	FleetView               bool   `json:"fleetView"`
	AgentMentions           string `json:"agentMentions"`
	RememberAgents          bool   `json:"rememberAgents"`
	WidgetMode              string `json:"widgetMode"`
	OutputTranscript        bool   `json:"outputTranscript"`
	WorktreeIsolation       bool   `json:"worktreeIsolation"`
	WorkflowsEnabled        bool   `json:"workflowsEnabled"`
	WorkflowsExplicit       bool   `json:"-"`
	ReportUsage             bool   `json:"reportUsage"`
	ShowCost                bool   `json:"showCost"`
	ShowModel               bool   `json:"showModel"`
	ViewerMarkdown          string `json:"viewerMarkdown"`
	FallbackSubagent        string `json:"fallbackSubagent"`
	ToolDescriptionMode     string `json:"toolDescriptionMode"`
	PackageAgents           any    `json:"packageAgents"`
	PackageWorkflows        any    `json:"packageWorkflows"`
}

func DefaultSettings() Settings {
	return Settings{MaxConcurrent: 10, GraceTurns: 5, MaxSubagentDepth: 2, DefaultJoinMode: "smart", BackgroundByDefault: true, SchedulingEnabled: true, FleetView: true, AgentMentions: "model", RememberAgents: true, WidgetMode: "background", OutputTranscript: true, WorktreeIsolation: true, WorkflowsEnabled: true, ViewerMarkdown: "assistant", FallbackSubagent: "general-purpose"}
}

func LoadSettings(cwd, agentDir string) (Settings, []string) {
	s := DefaultSettings()
	var warnings []string
	for _, path := range []string{filepath.Join(agentDir, "subagents.json"), filepath.Join(cwd, ".pi", "subagents.json")} {
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err == nil {
			candidate := s
			err = json.Unmarshal(b, &candidate)
			if err == nil {
				s = candidate
				var fields map[string]json.RawMessage
				if json.Unmarshal(b, &fields) == nil {
					if _, ok := fields["workflowsEnabled"]; ok {
						s.WorkflowsExplicit = true
					}
				}
			}
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("subagents: %s: %v", path, err))
		}
	}
	if s.MaxConcurrent < 1 || s.MaxConcurrent > 1024 {
		s.MaxConcurrent = 10
	}
	if s.MaxConcurrentForeground < 0 || s.MaxConcurrentForeground > 1024 {
		s.MaxConcurrentForeground = 0
	}
	if s.DefaultMaxTurns < 0 || s.DefaultMaxTurns > 10000 {
		s.DefaultMaxTurns = 0
	}
	if s.GraceTurns < 1 || s.GraceTurns > 1000 {
		s.GraceTurns = 5
	}
	if s.MaxSubagentDepth < 0 || s.MaxSubagentDepth > 16 {
		s.MaxSubagentDepth = 2
	}
	if !slices.Contains([]string{"all", "background", "off"}, s.WidgetMode) {
		s.WidgetMode = "background"
	}
	if !slices.Contains([]string{"model", "direct", "off"}, s.AgentMentions) {
		s.AgentMentions = "model"
	}
	if !slices.Contains([]string{"async", "group", "smart"}, s.DefaultJoinMode) {
		s.DefaultJoinMode = "smart"
	}
	if !slices.Contains([]string{"off", "assistant", "all"}, s.ViewerMarkdown) {
		s.ViewerMarkdown = "assistant"
	}
	return s, warnings
}

// SaveSettings patches only supplied keys, preserving unrelated extension fields.
func SaveSettings(cwd string, changes map[string]any) error {
	for key, v := range changes {
		switch key {
		case "maxConcurrent", "maxConcurrentForeground", "defaultMaxTurns", "graceTurns", "maxSubagentDepth":
			b, _ := json.Marshal(v)
			var n int
			if json.Unmarshal(b, &n) != nil {
				return fmt.Errorf("%s must be an integer", key)
			}
			lo, hi := 0, 10000
			switch key {
			case "maxConcurrent":
				lo, hi = 1, 1024
			case "maxConcurrentForeground":
				hi = 1024
			case "graceTurns":
				lo, hi = 1, 1000
			case "maxSubagentDepth":
				hi = 16
			}
			if n < lo || n > hi {
				return fmt.Errorf("%s must be between %d and %d", key, lo, hi)
			}
		case "widgetMode", "agentMentions", "viewerMarkdown", "defaultJoinMode":
			choices := map[string][]string{"widgetMode": {"all", "background", "off"}, "agentMentions": {"model", "direct", "off"}, "viewerMarkdown": {"off", "assistant", "all"}, "defaultJoinMode": {"smart", "group", "async"}}
			s, ok := v.(string)
			if !ok || !slices.Contains(choices[key], s) {
				return fmt.Errorf("invalid %s", key)
			}
		}
	}
	path := filepath.Join(cwd, ".pi", "subagents.json")
	raw := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err = json.Unmarshal(b, &raw); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for k, v := range changes {
		raw[k] = v
	}
	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".subagents-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

type Config struct {
	Name              string   `yaml:"name" json:"name"`
	DisplayName       string   `yaml:"display_name" json:"displayName"`
	Description       string   `yaml:"description" json:"description"`
	Color             string   `yaml:"color" json:"color,omitempty"`
	Tools             []string `yaml:"-" json:"tools,omitempty"`
	ExtSelectors      []string `yaml:"-" json:"extSelectors,omitempty"`
	DisallowedTools   []string `yaml:"-" json:"disallowedTools,omitempty"`
	Model             string   `yaml:"model" json:"model,omitempty"`
	Thinking          string   `yaml:"thinking" json:"thinking,omitempty"`
	MaxTurns          *int     `yaml:"max_turns" json:"maxTurns,omitempty"`
	InheritContext    *bool    `yaml:"inherit_context" json:"inheritContext,omitempty"`
	RunInBackground   *bool    `yaml:"run_in_background" json:"runInBackground,omitempty"`
	Isolated          *bool    `yaml:"isolated" json:"isolated,omitempty"`
	Isolation         string   `yaml:"isolation" json:"isolation,omitempty"`
	Memory            string   `yaml:"memory" json:"memory,omitempty"`
	AllowedSubagents  []string `yaml:"-" json:"allowedSubagents,omitempty"`
	Extensions        any      `yaml:"extensions" json:"extensions,omitempty"`
	ExcludeExtensions []string `yaml:"-" json:"excludeExtensions,omitempty"`
	Skills            any      `yaml:"skills" json:"skills,omitempty"`
	PersistSession    *bool    `yaml:"persist_session" json:"persistSession,omitempty"`
	SessionDir        string   `yaml:"session_dir" json:"sessionDir,omitempty"`
	OutputTranscript  *bool    `yaml:"output_transcript" json:"outputTranscript,omitempty"`
	PromptMode        string   `yaml:"prompt_mode" json:"promptMode"`
	SystemPrompt      string   `yaml:"-" json:"systemPrompt"`
	Enabled           *bool    `yaml:"enabled" json:"enabled,omitempty"`
	Source            string   `yaml:"-" json:"source"`
	SourcePath        string   `yaml:"-" json:"sourcePath,omitempty"`
}

var safeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
var safeHandle = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)

func stringList(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	var list []string
	switch x := v.(type) {
	case string:
		list = strings.Split(x, ",")
	case []any:
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string list")
			}
			list = append(list, s)
		}
	default:
		return nil, fmt.Errorf("expected CSV string or list")
	}
	out := []string{}
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func ParseConfig(path string, b []byte) (Config, error) {
	c := Config{Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), PromptMode: "replace", Extensions: true, Skills: true, SourcePath: path}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if strings.HasPrefix(text, "---\n") {
		i := strings.Index(text[4:], "\n---")
		if i < 0 {
			return c, fmt.Errorf("unclosed frontmatter")
		}
		i += 4
		front := text[4:i]
		if err := yaml.Unmarshal([]byte(front), &c); err != nil {
			return c, err
		}
		var raw map[string]any
		if err := yaml.Unmarshal([]byte(front), &raw); err != nil {
			return c, err
		}
		for key, dst := range map[string]*[]string{"tools": &c.Tools, "disallowed_tools": &c.DisallowedTools, "allowed_subagents": &c.AllowedSubagents, "exclude_extensions": &c.ExcludeExtensions} {
			if value, ok := raw[key]; ok {
				var err error
				*dst, err = stringList(value)
				if err != nil {
					return c, fmt.Errorf("%s: %w", key, err)
				}
			}
		}
		if v, ok := raw["disallowedTools"]; ok && raw["disallowed_tools"] == nil {
			var err error
			c.DisallowedTools, err = stringList(v)
			if err != nil {
				return c, err
			}
		}
		if raw["extensions"] == nil && raw["inherit_extensions"] != nil {
			c.Extensions = raw["inherit_extensions"]
		}
		if raw["skills"] == nil && raw["inherit_skills"] != nil {
			c.Skills = raw["inherit_skills"]
		}
		if v, ok := raw["maxTurns"]; ok && c.MaxTurns == nil {
			if n, ok := v.(int); ok {
				c.MaxTurns = &n
			}
		}
		text = strings.TrimPrefix(text[i+4:], "\n")
	}
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		c.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if len(c.Name) > 128 || strings.ContainsAny(c.Name, ":/\\\x00\r\n") || strings.TrimSpace(c.Name) == "" {
		return c, fmt.Errorf("unsafe agent name %q", c.Name)
	}
	if c.PromptMode != "append" && c.PromptMode != "replace" {
		return c, fmt.Errorf("prompt_mode must be append or replace")
	}
	if c.MaxTurns != nil && (*c.MaxTurns < 0 || *c.MaxTurns > 10000) {
		return c, fmt.Errorf("max_turns must be between 0 and 10000")
	}
	if c.Memory != "" && !slices.Contains([]string{"user", "project", "local"}, c.Memory) {
		return c, fmt.Errorf("invalid memory scope %q", c.Memory)
	}
	if c.Isolation != "" && c.Isolation != "off" && c.Isolation != "worktree" {
		return c, fmt.Errorf("invalid isolation %q", c.Isolation)
	}
	if c.Thinking != "" && !slices.Contains([]string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}, c.Thinking) {
		return c, fmt.Errorf("invalid thinking level %q", c.Thinking)
	}
	c.SystemPrompt = strings.TrimSpace(text)
	if c.Description == "" {
		c.Description = c.Name
	}
	var tools []string
	if c.Tools != nil {
		tools = []string{}
		for _, name := range c.Tools {
			if strings.HasPrefix(name, "ext:") {
				c.ExtSelectors = append(c.ExtSelectors, strings.TrimPrefix(name, "ext:"))
			} else {
				tools = append(tools, normalizeTool(name))
			}
		}
		c.Tools = tools
	}
	var denied []string
	for _, name := range c.DisallowedTools {
		denied = append(denied, name)
		if normalized := normalizeTool(name); normalized != name {
			denied = append(denied, normalized)
		}
	}
	c.DisallowedTools = denied
	if c.DisplayName == "" {
		c.DisplayName = c.Name
	}
	return c, nil
}

func normalizeTool(name string) string {
	switch strings.ToLower(name) {
	case "read":
		return "read"
	case "bash":
		return "bash"
	case "write":
		return "write"
	case "edit", "multiedit":
		return "edit"
	case "grep":
		return "grep"
	case "glob":
		return "find"
	case "ls":
		return "ls"
	}
	return name
}
func Inherits(value any, name string) bool {
	if value == nil {
		return true
	}
	if b, ok := value.(bool); ok {
		return b
	}
	list, err := stringList(value)
	return err == nil && slices.ContainsFunc(list, func(s string) bool { return strings.EqualFold(s, name) })
}

func LoadConfigs(cwd, agentDir string, settings Settings) ([]Config, []string) {
	configs := DefaultConfigs()
	if settings.DisableDefaultAgents {
		configs = nil
	}
	var warnings []string
	home, _ := os.UserHomeDir()
	for _, root := range []struct{ path, source string }{{filepath.Join(home, ".agents", "agents"), "global"}, {filepath.Join(agentDir, "agents"), "global"}, {filepath.Join(cwd, ".agents", "agents"), "project"}, {filepath.Join(cwd, ".pi", "agents"), "project"}} {
		files, err := os.ReadDir(root.path)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, err.Error())
			}
			continue
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
				continue
			}
			path := filepath.Join(root.path, file.Name())
			b, err := os.ReadFile(path)
			var c Config
			if err == nil {
				c, err = ParseConfig(path, b)
			}
			if err != nil {
				warnings = append(warnings, path+": "+err.Error())
				continue
			}
			c.Source = root.source
			if i := slices.IndexFunc(configs, func(v Config) bool { return v.Name == c.Name }); i >= 0 {
				configs[i] = c
			} else {
				configs = append(configs, c)
			}
		}
	}
	return configs, warnings
}

func resolveConfig(configs []Config, name, fallback string) (Config, string, error) {
	find := func(name string) (Config, bool) {
		var found []Config
		for _, c := range configs {
			if strings.EqualFold(c.Name, name) && (c.Enabled == nil || *c.Enabled) {
				found = append(found, c)
			}
		}
		if len(found) == 1 {
			return found[0], true
		}
		return Config{}, false
	}
	if c, ok := find(name); ok {
		return c, "", nil
	}
	if fallback == "" {
		fallback = "general-purpose"
	}
	if fallback != "none" {
		if c, ok := find(fallback); ok {
			return c, fmt.Sprintf("Agent type %q is unknown, disabled, or ambiguous; using %s.\n", name, c.Name), nil
		}
	}
	return Config{}, "", fmt.Errorf("agent type %q is unknown, disabled, or ambiguous", name)
}
