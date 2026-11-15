package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestEditRecovery(t *testing.T) {
	const source = "\ufeffpackage demo\r\nfunc alpha() {\r\n  call(\"old\")\r\n}\r\nfunc beta() {\r\n  call(\"old\")\r\n}\r\n"
	for _, test := range []struct {
		name  string
		edits []editEntry
		want  []string
	}{
		{"unmatched", []editEntry{{"package demo", "package changed"}, {"func alpha() {\n  missing()\n}", "replacement"}}, []string{"edits[1]", "candidate", "source.go:2"}},
		{"ambiguous", []editEntry{{"call(\"old\")", "replacement"}}, []string{"occurrences", "source.go:3", "source.go:6"}},
		{"overlap", []editEntry{{"func alpha() {", "replacement"}, {"alpha()", "other"}}, []string{"edits[0]", "edits[1]", "overlap"}},
		{"unrelated", []editEntry{{"nothing remotely similar", "replacement"}}, []string{"No credible candidate"}},
		{"blank", []editEntry{{" \n\t", "replacement"}}, []string{"No credible candidate"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "source.go")
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			tool := &EditTool{CWD: dir}
			args, _ := json.Marshal(editParams{Path: "source.go", Edits: test.edits})
			result, err := tool.Execute(context.Background(), "test", args, nil)
			if err != nil || !result.IsError {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			message := result.Content[0].(ai.TextContent).Text
			for _, want := range test.want {
				if !strings.Contains(message, want) {
					t.Errorf("missing %q in %s", want, message)
				}
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != source {
				t.Fatalf("rejected edit changed bytes: %q, %v", got, err)
			}
			args, _ = json.Marshal(editParams{Path: "source.go", Edits: []editEntry{{"func alpha() {\n  call(\"old\")\n}", "func alpha() {\n  call(\"new\")\n}"}, {"package demo", "package fixed"}}})
			result, err = tool.Execute(context.Background(), "corrected", args, nil)
			if err != nil || result.IsError {
				t.Fatalf("corrected result=%#v err=%v", result, err)
			}
			got, err = os.ReadFile(path)
			want := strings.Replace(strings.Replace(source, "package demo", "package fixed", 1), "call(\"old\")", "call(\"new\")", 1)
			if err != nil || string(got) != want {
				t.Fatalf("corrected bytes=%q, err=%v", got, err)
			}
		})
	}
}

func TestEditRecoveryDiagnosticsBounded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.go")
	source := strings.Repeat("target("+strings.Repeat("x", 1000)+")\n", 100)
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(editParams{Path: "source.go", Edits: []editEntry{{source[:1008], "replacement"}}})
	result, err := (&EditTool{CWD: dir}).Execute(context.Background(), "test", args, nil)
	if err != nil || !result.IsError {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	message := result.Content[0].(ai.TextContent).Text
	if !strings.Contains(message, "source.go:1") || len(message) > 1200 {
		t.Fatalf("unhelpful or unbounded diagnostic: %s", message)
	}
}
