package configroot

import (
	"path/filepath"
	"testing"
)

func TestDirPreservesSharedProfilePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, tc := range []struct {
		root, want string
	}{
		{"", filepath.Join(home, ".pi")},
		{"~", home},
		{"~/profile with spaces", filepath.Join(home, "profile with spaces")},
		{"~another-user", "~another-user"},
		{"relative", "relative"},
		{" profile with spaces ", " profile with spaces "},
		{" ", " "},
		{filepath.Join(home, "explicit"), filepath.Join(home, "explicit")},
	} {
		t.Run(tc.root, func(t *testing.T) {
			t.Setenv("PIG_HOME", tc.root)
			got, err := Dir()
			if err != nil || got != tc.want {
				t.Fatalf("Dir() = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}
