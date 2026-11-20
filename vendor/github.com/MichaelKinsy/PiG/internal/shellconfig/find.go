package shellconfig

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/internal/nodefs"
	"github.com/MichaelKinsy/PiG/internal/nodespawn"
)

// findTimeout is the time `which` or `where` gets, upstream's spawnSync timeout.
const findTimeout = 5 * time.Second

// FindExecutableOnPath uses where on Windows and which elsewhere. Windows Bash
// lookup selects the first existing native shell, excluding the WSL launcher.
// Other Windows lookups require the first result to exist. Unix output is trusted
// for Termux and special file systems. Failed or empty lookups return "".
//
// Ports packages/coding-agent/src/utils/shell.ts (findExecutableOnPath).
func FindExecutableOnPath(executable string) string {
	windows := runtime.GOOS == "windows"
	name := "which"
	if windows {
		name = "where"
	}
	ctx, cancel := context.WithTimeout(context.Background(), findTimeout)
	defer cancel()
	//nolint:gosec // G204 (class a): program is the literal which (where on Windows); the one argument is a fixed executable name such as bash or pwsh that PiG looks up.
	command := exec.CommandContext(ctx, name, executable)
	// Upstream's spawnSync pipes stdout and stderr, keeps stdin closed, and hides the Windows console window.
	nodespawn.HideWindow(command, nodespawn.Ignore, nodespawn.Pipe, nodespawn.Pipe)
	nodespawn.SetProgram(command)
	output, err := command.Output()
	if err != nil || len(output) == 0 {
		return ""
	}
	if windows && strings.EqualFold(executable, "bash.exe") {
		return FirstNativeBashMatch(string(output), nodefs.Access)
	}
	return FirstMatch(string(output), windows, nodefs.Access)
}

// FirstNativeBashMatch excludes the WSL launcher from automatic shell selection.
func FirstNativeBashMatch(output string, exists func(string) bool) string {
	for candidate := range strings.Lines(output) {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && !IsLegacyWSLBashPath(candidate) && exists(candidate) {
			return candidate
		}
	}
	return ""
}

// FirstMatch is the first line of the lookup command's output. On Windows the path must also exist according to exists,
// which upstream checks with accessSync(F_OK), not existsSync, so a runnable Microsoft Store alias is found.
func FirstMatch(output string, windows bool, exists func(string) bool) string {
	first, _, _ := strings.Cut(strings.TrimSpace(strings.ReplaceAll(output, "\r\n", "\n")), "\n")
	if first == "" {
		return ""
	}
	if windows && !exists(first) {
		return ""
	}
	return first
}
