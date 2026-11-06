package modelmetadata

// File-store plumbing: the snapshot file is read and written under Pi's directory lease, the way
// models-store.json is, so two processes never see a half-written catalog.

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/MichaelKinsy/PiG/internal/nodeerrno"
	"github.com/MichaelKinsy/PiG/internal/pilock"
)

// fileRevision identifies one on-disk version of the snapshot so an unchanged file is not re-read.
type fileRevision struct {
	modTime int64
	size    int64
}

// fileRevisionOf reports the current revision of path; found is false when the file does not exist.
func fileRevisionOf(path string) (fileRevision, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fileRevision{}, false, nil
		}
		return fileRevision{}, false, nodeerrno.FromPathError(err)
	}
	if info.IsDir() {
		return fileRevision{}, false, nil
	}
	return fileRevision{modTime: info.ModTime().UnixNano(), size: info.Size()}, true, nil
}

// withLockedFile runs fn with the file's contents under Pi's directory lease, creating the parent
// directory first. A missing file is reported as exists=false rather than as an error, because an
// unwritten snapshot is an empty catalog.
func withLockedFile(ctx context.Context, path string, fn func(content []byte, exists bool) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nodeerrno.FromPathError(err)
		}
	}
	lock, err := pilock.Acquire(ctx, path)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nodeerrno.FromPathError(err)
	}
	return fn(data, err == nil)
}

// writeLockedFile replaces path with data under Pi's directory lease. The write goes through a
// sibling temporary file so a crash never leaves a truncated snapshot behind.
func writeLockedFile(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nodeerrno.FromPathError(err)
		}
	}
	lock, err := pilock.Acquire(ctx, path)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	if err := ctx.Err(); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".models-metadata-*")
	if err != nil {
		return nodeerrno.FromPathError(err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() { _ = os.Remove(temporaryPath) }
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		cleanup()
		return nodeerrno.FromPathError(err)
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return nodeerrno.FromPathError(err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		cleanup()
		return nodeerrno.FromPathError(err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		cleanup()
		return nodeerrno.FromPathError(err)
	}
	return nil
}
