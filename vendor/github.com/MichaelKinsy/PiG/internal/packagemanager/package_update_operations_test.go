package packagemanager

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/internal/codingagent"
)

func TestUpdateReportsChangedGitPackage(t *testing.T) {
	t.Setenv("PI_OFFLINE", "")
	t.Setenv("PIG_OFFLINE", "")
	root := t.TempDir()
	origin := filepath.Join(root, "origin", "owner", "package.git")
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Dir(origin), 0o755); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, "init", "--bare", "--initial-branch=main", origin)
	gitTestCommand(t, "init", "--initial-branch=main", work)
	gitTestCommand(t, "-C", work, "config", "user.name", "Parity Test")
	gitTestCommand(t, "-C", work, "config", "user.email", "parity@example.invalid")
	gitTestCommand(t, "-C", work, "remote", "add", "origin", origin)
	writeGitFixtureFile(t, work, "version.txt", "one\n")
	gitTestCommand(t, "-C", work, "add", "version.txt")
	gitTestCommand(t, "-C", work, "commit", "-m", "first")
	gitTestCommand(t, "-C", work, "push", "-u", "origin", "main")

	cwd := filepath.Join(root, "project")
	agentDir := filepath.Join(root, "agent")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	repoURL := url.URL{Scheme: "file", Host: "localhost", Path: filepath.ToSlash(origin)}
	source := "git:" + repoURL.String()
	sm := codingagent.NewSettingsManager(cwd, agentDir)
	if err := InstallManagedGit(cwd, agentDir, sm, source, false); err != nil {
		t.Fatalf("initial package install: %v", err)
	}
	checkout, err := GitCheckoutPath(cwd, agentDir, source, false)
	if err != nil {
		t.Fatal(err)
	}
	oldHead := strings.TrimSpace(gitTestCommand(t, "-C", checkout, "rev-parse", "HEAD"))
	if err := sm.SetPackages([]codingagent.PackageSource{{Source: source}}); err != nil {
		t.Fatal(err)
	}

	writeGitFixtureFile(t, work, "version.txt", "two\n")
	gitTestCommand(t, "-C", work, "add", "version.txt")
	gitTestCommand(t, "-C", work, "commit", "-m", "second")
	gitTestCommand(t, "-C", work, "push", "origin", "main")
	wantHead := strings.TrimSpace(gitTestCommand(t, "-C", work, "rev-parse", "HEAD"))

	manager := NewPackageManager(PackageManagerOptions{CWD: cwd, SettingsManager: sm})
	var events []ProgressEvent
	manager.SetProgressCallback(func(event ProgressEvent) { events = append(events, event) })
	if err := manager.Update(""); err != nil {
		t.Fatalf("update package: %v", err)
	}

	gotHead := strings.TrimSpace(gitTestCommand(t, "-C", checkout, "rev-parse", "HEAD"))
	if gotHead != wantHead {
		t.Fatalf("updated checkout is at %s, want %s", gotHead, wantHead)
	}
	var results []ProgressEvent
	for _, event := range events {
		if event.Type == "result" && event.Action == "update" {
			results = append(results, event)
		}
	}
	if len(results) != 1 || results[0].Source != source || results[0].Message == nil {
		t.Fatalf("changed update results = %#v, want one result for %q", results, source)
	}
	if got, want := *results[0].Message, source+" "+oldHead[:7]+" -> "+wantHead[:7]; got != want {
		t.Fatalf("update summary = %q, want %q", got, want)
	}

	events = nil
	if err := manager.Update(""); err != nil {
		t.Fatalf("repeat update: %v", err)
	}
	for _, event := range events {
		if event.Type == "result" && event.Action == "update" {
			t.Fatalf("unchanged package produced update result: %#v", event)
		}
	}
}

func writeGitFixtureFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func gitTestCommand(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
