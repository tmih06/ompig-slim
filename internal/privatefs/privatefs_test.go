package privatefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsurePrivateDirCreatesAndRepeats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "private")
	for range 2 {
		if err := EnsurePrivateDir(path); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("path is not a directory: %s", path)
	}
}

func TestEnsurePrivateDirRejectsEmptyPath(t *testing.T) {
	if err := EnsurePrivateDir(""); err == nil {
		t.Fatal("accepted empty path")
	}
}

func TestSyncDir(t *testing.T) {
	path := t.TempDir()
	if err := SyncDir(path); err != nil {
		t.Fatal(err)
	}
}
