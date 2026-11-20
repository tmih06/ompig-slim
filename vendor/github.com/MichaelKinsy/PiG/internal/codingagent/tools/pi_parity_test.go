package tools

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
)

func parityExecute(t *testing.T, tool agent.AgentTool, params any) agent.AgentToolResult {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Execute(context.Background(), "parity-test", raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func parityWrite(t *testing.T, root, path string, content []byte) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func requireSearchBinary(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is required for this search-tool regression test", name)
	}
	return path
}

func requireFindBinary(t *testing.T) string {
	t.Helper()
	if path := LookupToolPath("fd", ""); path != "" {
		return path
	}
	t.Skip("fd is required for this search-tool regression test")
	return ""
}

func TestGrepGlobIsRelativeToSearchPathAndIgnoresDotSlash(t *testing.T) {
	rg := requireSearchBinary(t, "rg")
	cwd := t.TempDir()
	parityWrite(t, cwd, "repo/migrations/main/a.js", []byte("needle\n"))

	result := parityExecute(t, &GrepTool{CWD: cwd, RgPath: rg}, map[string]any{
		"path": "repo", "glob": "./migrations/main/*.js", "pattern": "needle",
	})
	if result.IsError || result.Text() != "migrations/main/a.js:1: needle" {
		t.Fatalf("grep result = %#v", result)
	}
}

func TestGrepContextComesFromRipgrepNotReadFile(t *testing.T) {
	rg := requireSearchBinary(t, "rg")
	cwd := t.TempDir()
	parityWrite(t, cwd, "match.txt", []byte("before\nneedle\nafter\n"))
	readFileCalled := false
	tool := &GrepTool{
		CWD:    cwd,
		RgPath: rg,
		Operations: &GrepOperations{
			IsDirectory: func(path string) (bool, error) { return false, nil },
			ReadFile: func(string) (string, error) {
				readFileCalled = true
				return "not the searched file\n", nil
			},
		},
	}

	result := parityExecute(t, tool, map[string]any{"path": "match.txt", "pattern": "needle", "context": 1})
	want := "match.txt-1- before\nmatch.txt:2: needle\nmatch.txt-3- after"
	if result.IsError || result.Text() != want {
		t.Fatalf("grep context = %q, want %q (error=%v)", result.Text(), want, result.IsError)
	}
	if readFileCalled {
		t.Fatal("GrepOperations.ReadFile was called for context")
	}
}

func TestGrepLimitNoticeRequiresAnAdditionalMatch(t *testing.T) {
	rg := requireSearchBinary(t, "rg")
	cwd := t.TempDir()
	parityWrite(t, cwd, "matches.txt", []byte("needle\n"))
	tool := &GrepTool{CWD: cwd, RgPath: rg}

	exact := parityExecute(t, tool, map[string]any{"path": "matches.txt", "pattern": "needle", "limit": 1})
	if exact.IsError || strings.Contains(exact.Text(), "matches limit reached") {
		t.Fatalf("exact-limit grep result = %#v", exact)
	}
	parityWrite(t, cwd, "matches.txt", []byte("needle\nneedle\n"))
	more := parityExecute(t, tool, map[string]any{"path": "matches.txt", "pattern": "needle", "limit": 1})
	if more.IsError || !strings.Contains(more.Text(), "1 matches limit reached") {
		t.Fatalf("over-limit grep result = %#v", more)
	}
}

func TestGrepLimitRetainsTrailingContextWithoutCountingIt(t *testing.T) {
	rg := requireSearchBinary(t, "rg")
	cwd := t.TempDir()
	parityWrite(t, cwd, "matches.txt", []byte("before\nneedle\nneedle\nafter\n"))
	tool := &GrepTool{CWD: cwd, RgPath: rg}

	result := parityExecute(t, tool, map[string]any{"path": "matches.txt", "pattern": "needle", "context": 1, "limit": 1})
	want := "matches.txt-1- before\nmatches.txt:2: needle\nmatches.txt-3- needle"
	if result.IsError || !strings.HasPrefix(result.Text(), want) || !strings.Contains(result.Text(), "1 matches limit reached") {
		t.Fatalf("grep limit with context = %#v, want trailing context and a match-limit notice", result)
	}
	if strings.Contains(result.Text(), "matches.txt-4-") {
		t.Fatalf("grep included context past the last allowed match: %q", result.Text())
	}
}

func TestFindPatternsAreAnchoredToSearchPath(t *testing.T) {
	fd := requireFindBinary(t)
	cwd := t.TempDir()
	parityWrite(t, cwd, "repo/src/main.ts", []byte(""))
	parityWrite(t, cwd, "repo/pkg/src/nested.ts", []byte(""))
	tool := &FindTool{CWD: cwd, FdPath: fd}

	for _, pattern := range []string{"src/*.ts", "./src/*.ts"} {
		result := parityExecute(t, tool, map[string]any{"path": "repo", "pattern": pattern})
		if result.IsError || result.Text() != "src/main.ts" {
			t.Errorf("find pattern %q = %#v, want only src/main.ts", pattern, result)
		}
	}
}

func TestFindLimitIsClampedAndNoticeRequiresMoreResults(t *testing.T) {
	fd := requireFindBinary(t)
	cwd := t.TempDir()
	parityWrite(t, cwd, "repo/src/main.ts", []byte(""))
	tool := &FindTool{CWD: cwd, FdPath: fd}

	for _, limit := range []float64{0, 1} {
		result := parityExecute(t, tool, map[string]any{"path": "repo/src", "pattern": "*.ts", "limit": limit})
		if result.IsError || result.Text() != "main.ts" {
			t.Errorf("find limit %v = %#v, want one result without a notice", limit, result)
		}
	}
	parityWrite(t, cwd, "repo/src/other.ts", []byte(""))
	result := parityExecute(t, tool, map[string]any{"path": "repo/src", "pattern": "*.ts", "limit": 1})
	if result.IsError || !strings.Contains(result.Text(), "1 results limit reached") {
		t.Fatalf("over-limit find = %#v", result)
	}
}

func TestLsLimitIsClampedAndNoticeRequiresMoreEntries(t *testing.T) {
	cwd := t.TempDir()
	parityWrite(t, cwd, "repo/src/main.ts", []byte(""))
	tool := &LsTool{CWD: cwd}

	for _, limit := range []float64{0, 1} {
		result := parityExecute(t, tool, map[string]any{"path": "repo/src", "limit": limit})
		if result.IsError || result.Text() != "main.ts" {
			t.Errorf("ls limit %v = %#v, want one result without a notice", limit, result)
		}
	}
	parityWrite(t, cwd, "repo/src/other.ts", []byte(""))
	result := parityExecute(t, tool, map[string]any{"path": "repo/src", "limit": 1})
	if result.IsError || !strings.Contains(result.Text(), "1 entries limit reached") {
		t.Fatalf("over-limit ls = %#v", result)
	}
}

func TestReadStreamMatchesWholeFileSelection(t *testing.T) {
	cases := []struct {
		name string
		data string
		args map[string]any
	}{
		{name: "selected range", data: "first\nsecond\nthird\nfourth", args: map[string]any{"path": "fixture.txt", "offset": 2, "limit": 2}},
		{name: "trailing newline", data: "first\n", args: map[string]any{"path": "fixture.txt"}},
		{name: "line cap", data: strings.Repeat("line\n", 2001), args: map[string]any{"path": "fixture.txt"}},
		{name: "byte cap on first line", data: strings.Repeat("x", DefaultMaxBytes+1) + "\nlast", args: map[string]any{"path": "fixture.txt"}},
		{name: "byte cap after complete lines", data: strings.Repeat(strings.Repeat("x", 40)+"\n", 1500), args: map[string]any{"path": "fixture.txt"}},
		{name: "empty file", data: "", args: map[string]any{"path": "fixture.txt"}},
		{name: "zero-line range", data: "first\nsecond", args: map[string]any{"path": "fixture.txt", "limit": 0}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			data := []byte(test.data)
			bufferTool := &ReadTool{Operations: &ReadOperations{
				Access:   func(string) error { return nil },
				ReadFile: func(string) ([]byte, error) { return data, nil },
			}}
			streamCalled := false
			streamTool := &ReadTool{Operations: &ReadOperations{
				Access:              func(string) error { return nil },
				ReadFile:            func(string) ([]byte, error) { t.Fatal("stream read fell back to ReadFile"); return nil, nil },
				DetectImageMimeType: func(string) (string, error) { return "", nil },
				CreateReadStream: func(string) (io.ReadCloser, error) {
					streamCalled = true
					return io.NopCloser(strings.NewReader(test.data)), nil
				},
			}}
			want := parityExecute(t, bufferTool, test.args)
			got := parityExecute(t, streamTool, test.args)
			if !streamCalled {
				t.Fatal("ReadOperations.CreateReadStream was not called")
			}
			if got.IsError || got.Text() != want.Text() {
				t.Fatalf("stream result = %q (error=%v), whole-file result = %q", got.Text(), got.IsError, want.Text())
			}
			gotDetails, _ := json.Marshal(got.Details)
			wantDetails, _ := json.Marshal(want.Details)
			if string(gotDetails) != string(wantDetails) {
				t.Fatalf("stream details = %s, whole-file details = %s", gotDetails, wantDetails)
			}
		})
	}
}

func TestReadLocalStreamPreservesInvalidUTF8AndBOM(t *testing.T) {
	cwd := t.TempDir()
	data := append([]byte("\uFEFFfirst\nsecond "), 0xff, '\n')
	parityWrite(t, cwd, "fixture.txt", data)
	result := parityExecute(t, &ReadTool{CWD: cwd}, map[string]any{"path": "fixture.txt", "offset": 2})
	if result.IsError || result.Text() != "second �\n" {
		t.Fatalf("read result = %q (error=%v), want replacement decoding without the first-line BOM", result.Text(), result.IsError)
	}
}

func TestEditPreservesUntouchedMixedLineEndings(t *testing.T) {
	for _, test := range []struct {
		name     string
		original string
		oldText  string
		newText  string
		want     string
	}{
		{name: "untouched mixed endings", original: "one\r\nbefore\ntwo\r\n", oldText: "before", newText: "after", want: "one\r\nafter\ntwo\r\n"},
		{name: "inserted lines use edited line ending", original: "one\r\nold\r\nkeep\n", oldText: "old", newText: "new\ninserted", want: "one\r\nnew\r\ninserted\r\nkeep\n"},
		{name: "classic Mac endings", original: "one\rtarget\rtwo", oldText: "target", newText: "new\nvalue", want: "one\rnew\rvalue\rtwo"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cwd := t.TempDir()
			parityWrite(t, cwd, "mixed.txt", []byte(test.original))
			result := parityExecute(t, &EditTool{CWD: cwd}, map[string]any{
				"path":  "mixed.txt",
				"edits": []map[string]string{{"oldText": test.oldText, "newText": test.newText}},
			})
			got, err := os.ReadFile(filepath.Join(cwd, "mixed.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError || string(got) != test.want {
				t.Fatalf("edit result = %#v, file = %q, want %q", result, got, test.want)
			}
		})
	}
}
