package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
)

// TestParseGitHubReference keeps URL routing separate from raw and file reads.
func TestParseGitHubReference(t *testing.T) {
	for _, test := range []struct {
		input      string
		kind       string
		repository string
		number     int
		ok         bool
	}{
		{input: "issue://123", kind: "issue", number: 123, ok: true},
		{input: "pr://owner/repo#42", kind: "pull", repository: "owner/repo", number: 42, ok: true},
		{input: "issue://owner/repo/7", kind: "issue", repository: "owner/repo", number: 7, ok: true},
		{input: "https://github.com/acme/widget/issues/7#issuecomment-123", kind: "issue", repository: "acme/widget", number: 7, ok: true},
		{input: "https://github.com/acme/widget/pull/42/", kind: "pull", repository: "acme/widget", number: 42, ok: true},
		{input: "https://github.com/acme/widget/issues/7?foo=bar", kind: "issue", repository: "acme/widget", number: 7, ok: true},
		{input: "https://example.com/acme/widget/issues/7", ok: false},
		{input: "https://github.com/acme/widget/issues/7:raw", ok: false},
		{input: "https://github.com/acme/widget/pull/42/files", ok: false},
		{input: "issue://0", ok: false},
		{input: "issue://not-a-number", ok: false},
	} {
		got, ok := parseGitHubReference(test.input)
		if ok != test.ok || ok && (got.Kind != test.kind || got.Repository != test.repository || got.Number != test.number) {
			t.Fatalf("parseGitHubReference(%q) = %#v, %v; want kind=%q repo=%q number=%d, %v", test.input, got, ok, test.kind, test.repository, test.number, test.ok)
		}
	}
}

// TestGitHubRepository checks discovery without git or explicit owner/repo arguments.
func TestGitHubRepository(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "")
	for _, layout := range []string{"checkout", "worktree-relative", "worktree-absolute", "submodule"} {
		t.Run(layout, func(t *testing.T) {
			root := t.TempDir()
			checkout := filepath.Join(root, "checkout")
			gitDir := filepath.Join(checkout, ".git")
			files := map[string]string{}
			if layout != "checkout" {
				gitDir = filepath.Join(root, "metadata", "worktrees", "feature")
				target := gitDir
				if layout != "worktree-absolute" {
					var err error
					target, err = filepath.Rel(checkout, gitDir)
					if err != nil {
						t.Fatal(err)
					}
				}
				files[filepath.Join(checkout, ".git")] = "gitdir: " + target + "\n"
				if layout != "submodule" {
					files[filepath.Join(gitDir, "commondir")] = "../..\n"
					gitDir = filepath.Join(root, "metadata")
				}
			}
			files[filepath.Join(gitDir, "config")] = "[remote \"origin\"]\n\turl=ssh://git@github.com/acme/widget.git\n"
			for path, content := range files {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cwd := filepath.Join(checkout, "src")
			if err := os.MkdirAll(cwd, 0700); err != nil {
				t.Fatal(err)
			}
			if got := githubRepository(cwd); got != "acme/widget" {
				t.Fatalf("repository = %q", got)
			}
			t.Chdir(cwd)
			if got := githubRepository(""); got != "acme/widget" {
				t.Fatalf("process cwd repository = %q", got)
			}
			t.Setenv("GITHUB_REPOSITORY", "override/project")
			if got := githubRepository(cwd); got != "override/project" {
				t.Fatalf("environment override repository = %q", got)
			}
		})
	}
}

// TestGitHubRepositoryStopsAtNearestCheckout avoids reading an enclosing repo's item.
func TestGitHubRepositoryStopsAtNearestCheckout(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "")
	root := t.TempDir()
	for _, dir := range []string{filepath.Join(root, ".git"), filepath.Join(root, "nested", ".git")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("[remote \"origin\"]\nurl = https://github.com/parent/project.git\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := githubRepository(filepath.Join(root, "nested")); got != "" {
		t.Fatalf("borrowed parent identity: %q", got)
	}
}

// TestReadGitHubAutoDetect reads both kinds from worktrees and external URLs.
func TestReadGitHubAutoDetect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/issues/7", "/repos/acme/widget/pulls/7":
			_, _ = w.Write([]byte(`{"title":"Detected","state":"open","body":"Description"}`))
		case "/repos/acme/widget/issues/7/comments":
			_, _ = w.Write([]byte(`[{"user":{"login":"octocat"},"body":"Discussion"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GITHUB_API_URL", server.URL)
	t.Setenv("GITHUB_REPOSITORY", "")
	root := t.TempDir()
	checkout := filepath.Join(root, "checkout")
	metadata := filepath.Join(root, "metadata")
	for _, dir := range []string{checkout, filepath.Join(metadata, "worktrees", "feature")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, contents := range map[string]string{
		filepath.Join(checkout, ".git"):                              "gitdir: ../metadata/worktrees/feature\n",
		filepath.Join(metadata, "worktrees", "feature", "commondir"): "../..\n",
		filepath.Join(metadata, "config"):                            "[remote \"origin\"]\nurl = git@github.com:acme/widget.git\n",
	} {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name string
		path string
	}{
		{name: "issue", path: "issue://7"},
		{name: "pr", path: "pr://7"},
		{name: "invocation-cwd", path: "issue://7"},
		{name: "process-cwd", path: "pr://7"},
		{name: "external-issue", path: "https://github.com/acme/widget/issues/7"},
		{name: "external-pr", path: "https://github.com/acme/widget/pull/7"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cwd := checkout
			ctx := context.Background()
			switch test.name {
			case "invocation-cwd":
				cwd = t.TempDir()
				ctx = extension.WithContext(ctx, extension.NewContext(checkout, nil, func() error { return nil }, extension.ContextActions{}))
			case "process-cwd":
				t.Chdir(checkout)
				cwd = ""
			}
			if strings.HasPrefix(test.path, "https:") {
				// External URLs must not need any checkout or inherit its identity.
				cwd = t.TempDir()
				t.Setenv("GITHUB_REPOSITORY", "unrelated/project")
			}
			params, err := json.Marshal(readParams{Path: test.path})
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&ReadTool{CWD: cwd}).Execute(ctx, "", params, nil)
			if err != nil || result.IsError {
				t.Fatalf("Execute = %#v, %v", result, err)
			}
			text := result.Content[0].(ai.TextContent).Text
			for _, want := range []string{"Detected", "Description", "Discussion"} {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q in %s", want, text)
				}
			}
			if details := result.Details.(*githubReadDetails); details.Repository != "acme/widget" {
				t.Fatalf("details = %#v", details)
			}
		})
	}
	result, err := (&ReadTool{CWD: t.TempDir()}).Execute(context.Background(), "", []byte(`{"path":"issue://7"}`), nil)
	if err != nil || !result.IsError || !strings.Contains(result.Content[0].(ai.TextContent).Text, "Cannot detect") {
		t.Fatalf("missing checkout result = %#v, %v", result, err)
	}
}

// TestReadGitHubIssueUsesCurrentRepositoryAndIncludesComments exercises the API path.
func TestReadGitHubIssueUsesCurrentRepositoryAndIncludesComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response any
		switch r.URL.Path {
		case "/repos/acme/widget/issues/7":
			response = map[string]any{"title": "Fix the widget", "state": "open", "body": "Please fix this."}
		case "/repos/acme/widget/issues/7/comments":
			response = []map[string]any{{"user": map[string]any{"login": "octocat"}, "body": "I can reproduce it."}}
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	t.Setenv("GITHUB_API_URL", server.URL)
	t.Setenv("GITHUB_REPOSITORY", "acme/widget")

	result, err := (&ReadTool{}).Execute(context.Background(), "", []byte(`{"path":"issue://7"}`), nil)
	if err != nil || result.IsError {
		t.Fatalf("read Execute returned err=%v error=%v", err, result.IsError)
	}
	text := result.Content[0].(ai.TextContent).Text
	for _, want := range []string{"# Issue #7: Fix the widget", "Please fix this.", "octocat", "I can reproduce it."} {
		if !strings.Contains(text, want) {
			t.Errorf("result does not contain %q:\n%s", want, text)
		}
	}
}
