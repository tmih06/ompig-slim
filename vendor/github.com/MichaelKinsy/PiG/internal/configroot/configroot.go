// Package configroot resolves the harness's writable root. PiG's explicit PIG_HOME
// override remains available for library-owned assets; the default is ~/.pi so
// the personal harness shares Pi and OMP's configuration.
package configroot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandTilde expands a leading ~ in a filesystem path, as upstream expandTildePath (utils/paths.ts normalizePath) does. It has no error result:
// when os.UserHomeDir fails, the home directory is the empty string. Node's os.homedir() instead falls back to the passwd database and throws when
// that fails too. A PiG-owned root uses [ExpandHome] instead, which reports the failure.
func ExpandTilde(path string) string {
	expanded, _ := expandHome(path, true)
	return expanded
}

// ExpandHome expands a leading ~ or ~/ in path and returns an error when the home directory is needed and unavailable. A path with no leading ~
// ("~user", a ~ elsewhere, an absolute or relative path) is returned unchanged and needs no home directory.
func ExpandHome(path string) (string, error) { return expandHome(path, false) }

func expandHome(path string, lenient bool) (string, error) {
	rest, ok := strings.CutPrefix(path, "~/")
	if path != "~" && !ok {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil && !lenient {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, rest), nil
}

// Resolve returns PIG_HOME when explicitly configured, otherwise ~/.pi.
// It never returns a relative fallback when the home directory is unavailable.
func Resolve() (string, error) {
	if v := os.Getenv("PIG_HOME"); v != "" {
		return ExpandHome(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".pi"), nil
}

// UnresolvedError is the value [Dir] panics with. The pig command recovers it, prints the message and exits 1.
type UnresolvedError struct{ Err error }

func (e *UnresolvedError) Error() string {
	return "cannot resolve ompig-slim's config directories: " + e.Err.Error() + "; set OMPIG_SLIM_CODING_AGENT_DIR"
}

func (e *UnresolvedError) Unwrap() error { return e.Err }

// Dir is [Resolve] for a caller that has no way to report a failure. It panics with an [*UnresolvedError] instead of returning a path relative to the
// working directory; the pig command reports that panic as an error and exits.
func Dir() string {
	root, err := Resolve()
	if err != nil {
		panic(&UnresolvedError{Err: err})
	}
	return root
}
