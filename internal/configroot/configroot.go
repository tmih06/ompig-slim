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

// Dir returns the configuration root: PIG_HOME with a leading ~ or ~/ expanded,
// as PiG's configroot.Resolve does, else ~/.pi. It reports an error only when
// the home directory is needed and cannot be resolved.
func Dir() (string, error) {
	root := os.Getenv("PIG_HOME")
	if root != "~" && !strings.HasPrefix(root, "~/") && root != "" {
		return root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if root == "~" {
		return home, nil
	}
	if root != "" {
		return filepath.Join(home, strings.TrimPrefix(root, "~/")), nil
	}
	return filepath.Join(home, ".pi"), nil
}
