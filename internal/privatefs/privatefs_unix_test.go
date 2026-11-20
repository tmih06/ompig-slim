//go:build !windows

package privatefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestrictFileTightensPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := file.Chmod(0644); err != nil {
		t.Fatal(err)
	}
	if err := RestrictFile(file); err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("file mode %o, want 600", mode)
	}
}

func TestEnsurePrivateDirTightensPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := EnsurePrivateDir(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0700 {
		t.Fatalf("directory mode %o, want 700", mode)
	}
}
