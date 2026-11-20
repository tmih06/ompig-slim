//go:build !windows

package selfupdate

import (
	"os"
	"path/filepath"
)

func lockUpdate(string) (func(), error) { return func() {}, nil }

func commitReplacement(staged, target string) error { return os.Rename(staged, target) }

func ownedExecutable(marked, running string) (string, bool) {
	if marked == "" || running == "" {
		return "", false
	}
	runningOriginal := running
	if resolved, err := filepath.EvalSymlinks(marked); err == nil {
		marked = resolved
	}
	if resolved, err := filepath.EvalSymlinks(running); err == nil {
		running = resolved
	}
	markedAbs, markedErr := filepath.Abs(marked)
	runningAbs, runningErr := filepath.Abs(running)
	if markedErr != nil || runningErr != nil {
		return runningOriginal, marked == running
	}
	return runningOriginal, markedAbs == runningAbs
}
