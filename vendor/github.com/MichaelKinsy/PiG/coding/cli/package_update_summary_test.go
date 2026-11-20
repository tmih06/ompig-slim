package cli

import (
	"bytes"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/packagemanager"
)

func TestRunPackageUpdatePrintsChangedPackageSummary(t *testing.T) {
	t.Setenv("PI_OFFLINE", "")
	t.Setenv("PIG_OFFLINE", "")
	root := t.TempDir()
	origin := filepath.Join(root, "origin", "owner", "package.git")
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Dir(origin), 0o755); err != nil {
		t.Fatal(err)
	}
	gitFixtureCommand(t, "init", "--bare", "--initial-branch=main", origin)
	gitFixtureCommand(t, "init", "--initial-branch=main", work)
	gitFixtureCommand(t, "-C", work, "config", "user.name", "Parity Test")
	gitFixtureCommand(t, "-C", work, "config", "user.email", "parity@example.invalid")
	gitFixtureCommand(t, "-C", work, "remote", "add", "origin", origin)
	writeGitFixture(t, work, "version.txt", "one\n")
	gitFixtureCommand(t, "-C", work, "add", "version.txt")
	gitFixtureCommand(t, "-C", work, "commit", "-m", "first")
	gitFixtureCommand(t, "-C", work, "push", "-u", "origin", "main")

	cwd := filepath.Join(root, "project")
	agentDir := filepath.Join(root, "agent")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	repoURL := url.URL{Scheme: "file", Host: "localhost", Path: filepath.ToSlash(origin)}
	source := "git:" + repoURL.String()
	sm := codingagent.NewSettingsManager(cwd, agentDir)
	if err := packagemanager.InstallManagedGit(cwd, agentDir, sm, source, false); err != nil {
		t.Fatalf("initial package install: %v", err)
	}
	checkout, err := packagemanager.GitCheckoutPath(cwd, agentDir, source, false)
	if err != nil {
		t.Fatal(err)
	}
	oldHead := strings.TrimSpace(gitFixtureCommand(t, "-C", checkout, "rev-parse", "HEAD"))
	if err := sm.SetPackages([]codingagent.PackageSource{{Source: source}}); err != nil {
		t.Fatal(err)
	}
	writeGitFixture(t, work, "version.txt", "two\n")
	gitFixtureCommand(t, "-C", work, "add", "version.txt")
	gitFixtureCommand(t, "-C", work, "commit", "-m", "second")
	gitFixtureCommand(t, "-C", work, "push", "origin", "main")
	newHead := strings.TrimSpace(gitFixtureCommand(t, "-C", work, "rev-parse", "HEAD"))

	var output bytes.Buffer
	if err := runPackageUpdate(&output, cwd, sm, ""); err != nil {
		t.Fatalf("run package update: %v", err)
	}
	for _, want := range []string{
		"Updating " + source + "...",
		"Updated 1 package:",
		"  " + source + " " + oldHead[:7] + " -> " + newHead[:7],
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("package update output %q does not contain %q", output.String(), want)
		}
	}
}

func TestPrintPackageUpdateSummary(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		updated  []string
		warnings int
		offline  bool
		want     []string
		omit     []string
	}{
		{
			name:    "changed packages",
			updated: []string{"npm:one 1.0.0 -> 1.1.0", "git:owner/repo 1234567 -> 7654321"},
			want:    []string{"Updated 2 packages:", "npm:one 1.0.0 -> 1.1.0", "git:owner/repo 1234567 -> 7654321"},
		},
		{
			name:    "one changed package",
			updated: []string{"npm:one 1.0.0 -> 1.1.0"},
			want:    []string{"Updated 1 package:", "npm:one 1.0.0 -> 1.1.0"},
		},
		{
			name:   "already current",
			source: "npm:one",
			want:   []string{"npm:one is already up to date"},
		},
		{
			name:     "warning without update",
			source:   "npm:one",
			warnings: 1,
			want:     []string{"npm:one was not updated, see the warnings above"},
			omit:     []string{"already up to date"},
		},
		{
			name: "no configured updates",
			want: []string{"Packages are already up to date"},
		},
		{
			name:     "offline skip takes precedence",
			source:   "npm:one",
			updated:  []string{"npm:one 1.0.0 -> 1.1.0"},
			warnings: 1,
			offline:  true,
			want:     []string{"Skipped package updates (offline mode)"},
			omit:     []string{"already up to date", "Updated 1 package"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			printPackageUpdateSummary(&output, test.source, test.updated, test.warnings, test.offline)
			got := output.String()
			for _, want := range test.want {
				if !strings.Contains(got, want) {
					t.Errorf("summary %q does not contain %q", got, want)
				}
			}
			for _, omit := range test.omit {
				if strings.Contains(got, omit) {
					t.Errorf("summary %q unexpectedly contains %q", got, omit)
				}
			}
		})
	}
}

func writeGitFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func gitFixtureCommand(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
