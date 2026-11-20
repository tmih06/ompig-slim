//go:build !windows

package broker

import (
	"os"
	"testing"
)

func assertPrivatePath(t testing.TB, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0700 {
		t.Fatalf("directory mode %o, want 700", mode)
	}
}

func assertPrivateFile(t testing.TB, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("file mode %o, want 600", mode)
	}
}

func makeWidePath(t testing.TB, path string) {
	t.Helper()
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
}
