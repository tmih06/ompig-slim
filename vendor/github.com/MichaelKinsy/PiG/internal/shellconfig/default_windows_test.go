package shellconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultFindsPerUserGitWithoutPATH(t *testing.T) {
	for _, subdir := range []string{"Programs", ""} {
		t.Run(subdir, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("ProgramFiles", filepath.Join(root, "machine"))
			t.Setenv("ProgramFiles(x86)", "")
			t.Setenv("LOCALAPPDATA", root)
			t.Setenv("PATH", "")
			shell := filepath.Join(root, subdir, "Git", "bin", "bash.exe")
			if err := os.MkdirAll(filepath.Dir(shell), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(shell, []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			config, err := Default()
			if err != nil || config.Path != shell || config.CommandTransport != "" {
				t.Fatalf("selected shell = %+v, error = %v, want %q", config, err, shell)
			}
		})
	}
}
