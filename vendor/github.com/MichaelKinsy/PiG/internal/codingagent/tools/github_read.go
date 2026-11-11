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

// parseGitHubReference recognizes the compact issue:// and pr:// references.
// A repository may be omitted when the current directory's origin identifies it.
func parseGitHubReference(value string) (githubReference, bool) {
	match := githubReferencePattern.FindStringSubmatch(strings.TrimSpace(value))
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
		ref.Repository = githubRepository(t.CWD)
	}
	if !githubRepositoryPattern.MatchString(ref.Repository) {
		return githubReadError("GitHub reference needs owner/repo (set GITHUB_REPOSITORY or use issue://owner/repo#123)"), true
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

func githubRepository(cwd string) string {
	if repository := os.Getenv("GITHUB_REPOSITORY"); repository != "" {
		return strings.TrimSuffix(strings.TrimSpace(repository), ".git")
	}
	for directory := cwd; directory != ""; directory = filepath.Dir(directory) {
		data, err := os.ReadFile(filepath.Join(directory, ".git", "config"))
		if err == nil {
			if repository := repositoryFromGitConfig(string(data)); repository != "" {
				return repository
			}
		}
		next := filepath.Dir(directory)
		if next == directory {
			break
		}
	}
	return ""
}

func repositoryFromGitConfig(config string) string {
	section := false
	for _, line := range strings.Split(config, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			section = strings.EqualFold(trimmed, "[remote \"origin\"]")
			continue
		}
		if section && strings.HasPrefix(trimmed, "url =") {
			remote := strings.TrimSpace(strings.TrimPrefix(trimmed, "url ="))
			if parsed, err := url.Parse(remote); err == nil && parsed.Host == "github.com" {
				return strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
			}
			if strings.HasPrefix(remote, "git@github.com:") {
				return strings.Trim(strings.TrimSuffix(strings.TrimPrefix(remote, "git@github.com:"), ".git"), "/")
			}
		}
	}
	return ""
}
