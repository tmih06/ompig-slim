//go:build windows

package shellconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MichaelKinsy/PiG/internal/nodefs"
)

// Default resolves bash on Windows, mirroring upstream getShellConfig's win32
// branch (shell.ts), including per-user Git for Windows installs, then bash.exe
// on PATH (Cygwin or MSYS2), else a helpful error. The WSL launcher is excluded.
// cmd.exe and PowerShell are not substitutes for bash syntax.
//
// Ports packages/coding-agent/src/utils/shell.ts (getShellConfig, win32 branch).
func Default() (Config, error) {
	var searched []string
	for _, p := range GitBashCandidates() {
		searched = append(searched, "  "+p)
		if nodefs.Access(p) {
			return ForBash(p), nil
		}
	}
	if p := FindExecutableOnPath("bash.exe"); p != "" {
		return ForBash(p), nil
	}
	return Config{}, fmt.Errorf(
		"No bash shell found. Options:\n"+
			"  1. Install Git for Windows: https://git-scm.com/download/win\n"+
			"  2. Add native bash to PATH (Cygwin or MSYS2, not WSL)\n"+
			"  3. Set shellPath in settings.json\n\n"+
			"Searched Git Bash in:\n%s", strings.Join(searched, "\n"))
}

// GitBashCandidates lists the standard machine and per-user native Git shells.
func GitBashCandidates() []string {
	var candidates []string
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if base != "" {
			candidates = append(candidates, filepath.Join(base, "Git", "bin", "bash.exe"))
		}
	}
	if base := os.Getenv("LOCALAPPDATA"); base != "" {
		candidates = append(candidates,
			filepath.Join(base, "Programs", "Git", "bin", "bash.exe"),
			filepath.Join(base, "Git", "bin", "bash.exe"))
	}
	return candidates
}
