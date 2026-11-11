package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestAttachmentSearchLiteralNames checks real fd searches rather than a mock
// that would hide flag parsing and smart-case filtering before ranking.
func TestAttachmentSearchLiteralNames(t *testing.T) {
	fd, err := exec.LookPath("fd")
	if err != nil {
		fd, err = exec.LookPath("fdfind")
	}
	if err != nil {
		t.Skip("fd is not installed")
	}
	dir := t.TempDir()
	for _, name := range []string{"README.md", "-notes.txt", "my notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := NewCombinedProvider(nil, dir, fd)
	p.SetAsyncFileSearch(true)
	for _, tc := range []struct{ query, want string }{
		{"@Readme", "@README.md"}, {"@-notes", "@-notes.txt"}, {`@"my no`, `@"my notes.txt"`},
	} {
		t.Run(tc.query, func(t *testing.T) {
			prefix, task, ok := p.SuggestionTask([]string{tc.query}, 0, len(tc.query), false)
			if !ok {
				t.Fatal("missing attachment search")
			}
			items, err := task(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range items {
				if item.Value == tc.want {
					lines, _, _ := p.ApplyCompletion([]string{"see " + tc.query}, 0, len("see "+tc.query), item, prefix)
					if lines[0] != "see "+tc.want+" " {
						t.Fatalf("completion = %q", lines[0])
					}
					return
				}
			}
			t.Fatalf("attachment %q missing from %+v", tc.want, items)
		})
	}
}
