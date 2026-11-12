package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

type githubReference struct {
	Kind       string
	Repository string
	Number     int
}

var githubReferencePattern = regexp.MustCompile(`^(issue|pr)://(?:([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)(?:#|/))?([1-9][0-9]*)$`)

// parseGitHubReference recognizes compact references and github.com issue/PR URLs.
// URL query strings and fragments identify the same item; other URL paths and
// read's :raw/:line-range suffixes remain with the ordinary URL reader.
func parseGitHubReference(value string) (githubReference, bool) {
	value = strings.TrimSpace(value)
	if parsed, err := url.Parse(value); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && strings.EqualFold(parsed.Host, "github.com") {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) == 4 && (parts[2] == "issues" || parts[2] == "pull") {
			kind := "issue"
			if parts[2] == "pull" {
				kind = "pr"
			}
			value = kind + "://" + parts[0] + "/" + parts[1] + "/" + parts[3]
		}
	}
	match := githubReferencePattern.FindStringSubmatch(value)
	if len(match) == 0 {
		return githubReference{}, false
	}
	number, err := strconv.Atoi(match[3])
	if err != nil {
		return githubReference{}, false
	}
	kind := match[1]
	if kind == "pr" {
		kind = "pull"
	}
	return githubReference{Kind: kind, Repository: match[2], Number: number}, true
}

type githubReadDetails struct {
	Kind       string `json:"kind"`
	Repository string `json:"repository"`
	Number     int    `json:"number"`
}

// readGitHub resolves and renders one public GitHub issue or pull request.
// It uses the REST API rather than shelling out to gh; GITHUB_TOKEN or GH_TOKEN
// is forwarded for private repositories, while unauthenticated public reads work.
func (t *ReadTool) readGitHub(ctx context.Context, path string) (agent.AgentToolResult, bool) {
	ref, ok := parseGitHubReference(path)
	if !ok {
		return agent.AgentToolResult{}, false
	}
	if ref.Repository == "" {
		cwd, err := toolCWD(ctx, t.CWD)
		if err != nil {
			return githubReadError(err.Error()), true
		}
		ref.Repository = githubRepository(cwd)
	}
	if !githubRepositoryPattern.MatchString(ref.Repository) {
		return githubReadError("Cannot detect a GitHub repository: run inside a checkout with a GitHub origin, provide a GitHub issue/PR URL, or set GITHUB_REPOSITORY"), true
	}
	base := strings.TrimRight(os.Getenv("GITHUB_API_URL"), "/")
	if base == "" {
		base = "https://api.github.com"
	}
	apiPath := "/repos/" + ref.Repository + "/issues/"
	if ref.Kind == "pull" {
		apiPath = "/repos/" + ref.Repository + "/pulls/"
	}
	var item map[string]any
	if err := githubGET(ctx, base+apiPath+strconv.Itoa(ref.Number), &item); err != nil {
		return githubReadError(err.Error()), true
	}
	text := formatGitHubItem(ref, item)
	if comments, err := githubComments(ctx, base, ref); err == nil && len(comments) > 0 {
		text += "\n\n## Comments\n" + formatGitHubComments(comments)
	}
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: text}},
		Details: &githubReadDetails{Kind: ref.Kind, Repository: ref.Repository, Number: ref.Number},
	}, true
}

var githubRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func githubReadError(message string) agent.AgentToolResult {
	return agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "Error: " + message}}, IsError: true}
}

func githubGET(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if token := os.Getenv("GH_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("GitHub API returned %s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

func githubComments(ctx context.Context, base string, ref githubReference) ([]map[string]any, error) {
	path := "/repos/" + ref.Repository + "/issues/" + strconv.Itoa(ref.Number) + "/comments?per_page=100"
	var comments []map[string]any
	return comments, githubGET(ctx, base+path, &comments)
}

func formatGitHubItem(ref githubReference, item map[string]any) string {
	title := stringValue(item, "title")
	body := stringValue(item, "body")
	state := stringValue(item, "state")
	kind := "Issue"
	if ref.Kind == "pull" {
		kind = "Pull Request"
	}
	text := fmt.Sprintf("# %s #%d: %s\n\nState: %s", kind, ref.Number, title, state)
	if body != "" {
		text += "\n\n" + body
	} else {
		text += "\n\n(No description)"
	}
	return text
}

func formatGitHubComments(comments []map[string]any) string {
	var out []string
	for _, comment := range comments {
		author := "unknown"
		if user, ok := comment["user"].(map[string]any); ok {
			author = stringValue(user, "login")
		}
		out = append(out, fmt.Sprintf("### %s\n\n%s", author, stringValue(comment, "body")))
	}
	return strings.Join(out, "\n\n")
}

func stringValue(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

// githubRepository uses the environment override, then the nearest checkout's
// origin. Git indirection and commondir support worktrees and submodules without
// requiring git on PATH. Never borrow a parent repository's identity.
func githubRepository(cwd string) string {
	if repository := strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY")); repository != "" {
		return strings.TrimSuffix(repository, ".git")
	}
	directory, err := filepath.Abs(cwd)
	if err != nil {
		return ""
	}
	for {
		gitDir := filepath.Join(directory, ".git")
		if info, err := os.Stat(gitDir); err == nil {
			if !info.IsDir() {
				data, err := os.ReadFile(gitDir)
				if err != nil {
					return ""
				}
				target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
				if !ok {
					return ""
				}
				gitDir = githubGitPath(directory, target)
			}
			if common, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
				gitDir = githubGitPath(gitDir, string(common))
			}
			data, _ := os.ReadFile(filepath.Join(gitDir, "config"))
			return repositoryFromGitConfig(string(data))
		}
		next := filepath.Dir(directory)
		if next == directory {
			return ""
		}
		directory = next
	}
}

// githubGitPath resolves Git metadata paths relative to the file's directory.
func githubGitPath(base, target string) string {
	target = strings.TrimSpace(target)
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Join(base, target)
}

// repositoryFromGitConfig extracts a GitHub origin from standard HTTPS/SSH URLs.
// It returns empty for missing or non-GitHub origins rather than guessing.
func repositoryFromGitConfig(config string) string {
	section := false
	for _, line := range strings.Split(config, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			section = strings.EqualFold(trimmed, "[remote \"origin\"]")
			continue
		}
		key, value, hasValue := strings.Cut(trimmed, "=")
		if section && hasValue && strings.EqualFold(strings.TrimSpace(key), "url") {
			remote := strings.Trim(strings.TrimSpace(value), "\"")
			if parsed, err := url.Parse(remote); err == nil && strings.EqualFold(parsed.Hostname(), "github.com") {
				return strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
			}
			if strings.HasPrefix(remote, "git@github.com:") {
				return strings.Trim(strings.TrimSuffix(strings.TrimPrefix(remote, "git@github.com:"), ".git"), "/")
			}
		}
	}
	return ""
}
