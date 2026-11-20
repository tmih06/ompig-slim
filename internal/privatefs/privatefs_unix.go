//go:build !windows

package privatefs

import "os"

// EnsurePrivateDir creates path and restricts it to its owner.
func EnsurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

// RestrictFile limits an opened file to the current user.
func RestrictFile(file *os.File) error {
	return file.Chmod(0600)
}

// SyncDir flushes directory entries to stable storage.
func SyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
