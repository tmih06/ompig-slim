package subagents

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func readTranscript(path string) ([]json.RawMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 16*1024*1024)
	var out []json.RawMessage
	for s.Scan() {
		var e struct {
			Type    string          `json:"type"`
			Message json.RawMessage `json:"message"`
		}
		if json.Unmarshal(s.Bytes(), &e) == nil && e.Type == "message" && len(e.Message) > 0 {
			out = append(out, e.Message)
		}
	}
	return out, s.Err()
}

type worktree struct{ path, branch, base, workPath string }

func git(ctx context.Context, cwd string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}
func createWorktree(ctx context.Context, cwd, id string) (*worktree, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	base, err := git(ctx, cwd, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	root, err := git(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(os.TempDir(), "pi-agent-"+id+"-"+newID(""))
	if _, err = git(ctx, cwd, "worktree", "add", "--detach", path, "HEAD"); err != nil {
		return nil, err
	}
	return &worktree{path: path, branch: "pi-agent-" + id, base: base, workPath: filepath.Join(path, rel)}, nil
}
func cleanupWorktree(cwd string, wt *worktree, description string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	status, err := git(ctx, wt.path, "status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("worktree retained at %s: %w", wt.path, err)
	}
	if status != "" {
		if _, err = git(ctx, wt.path, "add", "-A"); err == nil {
			_, err = git(ctx, wt.path, "commit", "-m", "pi-agent: "+clip(description, 200))
		}
		if err != nil {
			return "", fmt.Errorf("uncommitted work retained at %s: %w", wt.path, err)
		}
	}
	head, err := git(ctx, wt.path, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	branch := ""
	if head != wt.base {
		branch = wt.branch
		if _, err = git(ctx, wt.path, "branch", branch); err != nil {
			branch += "-" + newID("")
			if _, err = git(ctx, wt.path, "branch", branch); err != nil {
				return "", fmt.Errorf("worktree retained at %s: %w", wt.path, err)
			}
		}
	}
	if _, err = git(ctx, cwd, "worktree", "remove", wt.path); err != nil {
		return branch, fmt.Errorf("worktree retained at %s: %w", wt.path, err)
	}
	return branch, nil
}

// MemoryPrompt uses the extension's scope paths and bounded MEMORY.md excerpt.
// The tools of the selected agent determine whether the directory may be created.
func MemoryPrompt(cwd, agentDir string, c Config) (string, error) {
	if c.Memory == "" {
		return "", nil
	}
	if !safeName.MatchString(c.Name) {
		return "", fmt.Errorf("unsafe memory name")
	}
	root := filepath.Join(cwd, ".pi", "agent-memory")
	if c.Memory == "local" {
		root = filepath.Join(cwd, ".pi", "agent-memory-local")
	}
	if c.Memory == "user" {
		root = filepath.Join(agentDir, "agent-memory")
	}
	dir := filepath.Join(root, c.Name)
	for p := dir; ; p = filepath.Dir(p) {
		if st, err := os.Lstat(p); err == nil && st.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing symlinked memory path %s", p)
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	writable := c.Tools == nil
	for _, tool := range c.Tools {
		if tool == "write" || tool == "edit" {
			writable = true
		}
	}
	for _, tool := range c.DisallowedTools {
		if tool == "write" || tool == "edit" {
			writable = false
		}
	}
	if writable {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", err
		}
	}
	path := filepath.Join(dir, "MEMORY.md")
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) > 200 {
		lines = lines[:200]
	}
	mode := "Read existing memory; this agent has no memory write tools."
	if writable {
		mode = "Keep durable discoveries in MEMORY.md. Put detailed notes in separate files and link them from MEMORY.md."
	}
	return fmt.Sprintf("\n\n# Persistent agent memory\nMemory directory: %s\n%s\n\n%s", dir, mode, strings.Join(lines, "\n")), nil
}
