// Package configroot resolves the per-user configuration root this program
// shares with PiG and Pi.
//
// The root is the directory that holds `settings.json`, `sessions/`, `state/`,
// and the install marker. Everything in this repository that must find one of
// those uses this package, so a change to the resolution rule (a new
// environment variable, a platform default) happens once.
package configroot

import (
	"os"
	"path/filepath"
	"strings"
)

// Dir returns the configuration root: PIG_HOME when it is set and non-empty,
// else ~/.pi. It reports an error only when the home directory cannot be
// resolved, which is the one case where no useful fallback exists.
func Dir() (string, error) {
	if root := strings.TrimSpace(os.Getenv("PIG_HOME")); root != "" {
		return root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pi"), nil
}
