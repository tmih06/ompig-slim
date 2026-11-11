package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

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
		{input: "issue://0", ok: false},
		{input: "issue://not-a-number", ok: false},
	} {
		got, ok := parseGitHubReference(test.input)
		if ok != test.ok || ok && (got.Kind != test.kind || got.Repository != test.repository || got.Number != test.number) {
			t.Fatalf("parseGitHubReference(%q) = %#v, %v; want kind=%q repo=%q number=%d, %v", test.input, got, ok, test.kind, test.repository, test.number, test.ok)
		}
	}
}

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
