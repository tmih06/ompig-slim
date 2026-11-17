package streamingrules

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed builtin/*.md
var builtins embed.FS

type settings struct {
	enabled  bool
	builtin  bool
	disabled map[string]bool
}

func readSettings(values map[string]any) settings {
	result := settings{enabled: true, builtin: true, disabled: map[string]bool{}}
	values, _ = values["ttsr"].(map[string]any)
	if enabled, ok := values["enabled"].(bool); ok {
		result.enabled = enabled
	}
	if builtin, ok := values["builtinRules"].(bool); ok {
		result.builtin = builtin
	}
	if names, ok := values["disabledRules"].([]any); ok {
		for _, name := range names {
			if name, ok := name.(string); ok {
				result.disabled[name] = true
			}
		}
	}
	return result
}

func loadRules(cwd, agentDir string, config settings) ([]*rule, []string) {
	if !config.enabled {
		return nil, nil
	}
	var rules []*rule
	var warnings []string
	seen := map[string]bool{}
	add := func(name, file string, data []byte, builtin bool) {
		if seen[name] || config.disabled[name] {
			return
		}
		r, disabled, err := parseRule(name, file, string(data), builtin)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %s", file, err))
			return
		}
		if disabled || r != nil {
			seen[name] = true
		}
		if r != nil {
			rules = append(rules, r)
		}
	}
	var directories []string
	for dir := filepath.Clean(cwd); ; dir = filepath.Dir(dir) {
		for _, configDir := range []string{".pi", ".agents", ".omp", ".cursor"} {
			directories = append(directories, filepath.Join(dir, configDir, "rules"))
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil || filepath.Dir(dir) == dir {
			break
		}
	}
	directories = append(directories, filepath.Join(agentDir, "rules"))
	for _, directory := range directories {
		entries, err := os.ReadDir(directory)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, fmt.Sprintf("cannot read %s", directory))
			}
			continue
		}
		for _, entry := range entries {
			ext := filepath.Ext(entry.Name())
			if entry.IsDir() || (ext != ".md" && ext != ".mdc") {
				continue
			}
			file := filepath.Join(directory, entry.Name())
			data, err := os.ReadFile(file)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("cannot read %s", file))
				continue
			}
			add(strings.TrimSuffix(entry.Name(), ext), file, data, false)
		}
	}
	if config.builtin {
		files, _ := fs.Glob(builtins, "builtin/*.md")
		for _, file := range files {
			data, _ := builtins.ReadFile(file)
			add(strings.TrimSuffix(filepath.Base(file), ".md"), file, data, true)
		}
	}
	return rules, warnings
}
