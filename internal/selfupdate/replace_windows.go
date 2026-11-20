package selfupdate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"golang.org/x/sys/windows"
)

func lockUpdate(target string) (func(), error) {
	// The PowerShell installer locks the same byte (offset 0, length 1).
	lock := flock.New(target + ".update.lock")
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock update for %s: %w", target, err)
	}
	if !locked {
		return nil, fmt.Errorf("another install or update is in progress for %s", target)
	}
	return func() { _ = lock.Unlock() }, nil
}

func previousExecutable(target string) string {
	return strings.TrimSuffix(target, filepath.Ext(target)) + ".previous.exe"
}

func commitReplacement(staged, target string) error {
	return replaceWindowsExecutable(staged, target, moveWithoutOverwrite)
}

func replaceWindowsExecutable(staged, target string, move func(string, string) error) error {
	backup := previousExecutable(target)
	if info, err := os.Lstat(target); err != nil {
		if _, backupErr := os.Lstat(backup); os.IsNotExist(err) && backupErr == nil {
			return fmt.Errorf("executable is missing; recover it from %q: %w", backup, err)
		}
		return fmt.Errorf("inspect installed executable: %w", err)
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("installed executable %q is not a regular file", target)
	}
	if info, err := os.Lstat(backup); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("update backup %q is not a regular file", backup)
		}
		if err := retryWindowsFileMutation(func() error { return os.Remove(backup) }); err != nil {
			return fmt.Errorf("remove previous executable %q (close running copies and retry): %w", backup, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect previous executable: %w", err)
	}
	if err := move(target, backup); err != nil {
		return fmt.Errorf("move installed executable to %q: %w", backup, err)
	}
	if err := move(staged, target); err != nil {
		replaceErr := fmt.Errorf("install staged executable: %w", err)
		if rollbackErr := move(backup, target); rollbackErr != nil {
			return errors.Join(replaceErr, fmt.Errorf("restore failed; recover the previous executable from %q: %w", backup, rollbackErr))
		}
		return replaceErr
	}
	// The old image may still be mapped. Delete it only on a later locked run.
	return nil
}

func moveWithoutOverwrite(from, to string) error {
	from, err := extendedWindowsPath(from)
	if err != nil {
		return err
	}
	to, err = extendedWindowsPath(to)
	if err != nil {
		return err
	}
	source, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	destination, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return retryWindowsFileMutation(func() error {
		return windows.MoveFileEx(source, destination, 0)
	})
}

func retryWindowsFileMutation(mutate func() error) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := mutate()
		if (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_LOCK_VIOLATION)) ||
			!time.Now().Before(deadline) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func extendedWindowsPath(name string) (string, error) {
	stripped := normalizedWindowsPath(name)
	if stripped == name && strings.HasPrefix(name, `\\?\`) {
		return name, nil
	}
	name = stripped
	if !filepath.IsAbs(name) {
		absolute, err := filepath.Abs(name)
		if err != nil {
			return "", err
		}
		name = absolute
	}
	name = filepath.Clean(name)
	if strings.HasPrefix(name, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(name, `\\`), nil
	}
	return `\\?\` + name, nil
}

func normalizedWindowsPath(name string) string {
	if strings.HasPrefix(strings.ToLower(name), `\\?\unc\`) {
		return `\\` + name[len(`\\?\UNC\`):]
	}
	if strings.HasPrefix(name, `\\?\`) && len(name) >= 7 && name[5] == ':' && name[6] == '\\' {
		return name[4:]
	}
	return name
}

func resolveWindowsPath(name string) (string, error) {
	longPath, err := extendedWindowsPath(name)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(longPath)
	if err != nil {
		return "", err
	}
	return normalizedWindowsPath(resolved), nil
}

func statWindowsPath(name string) (os.FileInfo, error) {
	longPath, err := extendedWindowsPath(name)
	if err != nil {
		return nil, err
	}
	return os.Stat(longPath)
}

func ownedExecutable(marked, running string) (string, bool) {
	if !filepath.IsAbs(marked) || !filepath.IsAbs(running) {
		return "", false
	}
	marked, markedErr := resolveWindowsPath(marked)
	running, runningErr := resolveWindowsPath(running)
	if markedErr != nil || runningErr != nil || !strings.EqualFold(marked, running) {
		return "", false
	}
	markedInfo, markedErr := statWindowsPath(marked)
	runningInfo, runningErr := statWindowsPath(running)
	if markedErr != nil || runningErr != nil || !markedInfo.Mode().IsRegular() || !os.SameFile(markedInfo, runningInfo) {
		return "", false
	}
	return marked, true
}
