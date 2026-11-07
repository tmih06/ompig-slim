package subagents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// EjectConfig creates a project-owned copy. Installed package files are always
// read-only; disabling a package type is an explicit local enabled:false copy.
func (m *Manager) EjectConfig(c Config, disabled bool) (string, error) {
	if !safeName.MatchString(c.Name) || c.SourcePath == "" {
		return "", fmt.Errorf("agent cannot be ejected")
	}
	b, err := os.ReadFile(c.SourcePath)
	if err != nil {
		return "", err
	}
	body := strings.ReplaceAll(string(b), "\r\n", "\n")
	if disabled {
		parts := strings.SplitN(body, "---", 3)
		if len(parts) != 3 || strings.TrimSpace(parts[0]) != "" {
			return "", fmt.Errorf("agent frontmatter missing")
		}
		fields := map[string]any{}
		if err = yaml.Unmarshal([]byte(parts[1]), &fields); err != nil {
			return "", err
		}
		fields["enabled"] = false
		header, e := yaml.Marshal(fields)
		if e != nil {
			return "", e
		}
		body = "---\n" + string(header) + "---" + parts[2]
	}
	path := filepath.Join(m.host.CWD, ".pi", "agents", c.Name+".md")
	if _, err = ParseConfig(path, []byte(body)); err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, err = file.WriteString(body)
	closeErr := file.Close()
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	return path, m.Update(map[string]any{})
}
