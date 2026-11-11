// pig divergence (D2): refuse the `pi` name to protect Pi's command identity.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MichaelKinsy/PiG/internal/linkerexec"
)

// guardBinaryIdentity rejects Pi's binary name and reports suspect install paths.
func guardBinaryIdentity() string {
	argv0 := os.Args[0]
	resolved, err := linkerexec.Executable()
	if err != nil {
		resolved = argv0
	}
	resolved, _ = filepath.EvalSymlinks(resolved)

	base := filepath.Base(resolved)
	if base == "pi" || base == "pi.exe" {
		fmt.Fprintf(os.Stderr, `ompig-slim: refusing to run as %q.

This binary is ompig-slim, a Go coding agent built on PiG, not upstream pi.
Running it as "pi" would silently shadow the real pi binary and corrupt
shared expectations (PATH, sessions, ~/.pi/ tooling, shell aliases).

Resolved path: %s

If you want ompig-slim: rename or symlink it back to "ompig-slim".
If you want pi:  install via npm/mise; do not point "pi" at this file.
`, base, resolved)
		os.Exit(2)
	}

	// Warn when the binary is installed inside Pi's npm package.
	if strings.Contains(resolved, "/npm-mariozechner-pi-coding-agent/") ||
		strings.Contains(resolved, "/@mariozechner/pi-coding-agent/") ||
		strings.Contains(resolved, "/npm-earendil-works-pi-coding-agent/") ||
		strings.Contains(resolved, "/@earendil-works/pi-coding-agent/") {
		fmt.Fprintf(os.Stderr, "ompig-slim: warning: running from upstream pi install path: %s\n", resolved)
	}
	return resolved
}
