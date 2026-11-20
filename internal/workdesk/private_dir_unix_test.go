//go:build !windows

package workdesk_test

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
	perm := info.Mode().Perm()
	if info.IsDir() && perm != 0700 || !info.IsDir() && perm&0077 != 0 {
		t.Fatalf("path is not private: %s (%o)", path, perm)
	}
}

func makeWidePath(t testing.TB, path string) {
	t.Helper()
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func assertWidePath(t testing.TB, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 == 0 {
		t.Fatalf("path unexpectedly private: %s (%o)", path, info.Mode().Perm())
	}
}
