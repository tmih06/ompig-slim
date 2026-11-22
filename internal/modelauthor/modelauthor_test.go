package modelauthor

import (
	"cmp"
	"os/exec"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
)

func TestAuthorForFollowsTheModelNotTheProvider(t *testing.T) {
	const claude = "noreply@anthropic.com"
	const gemini = "176961590+gemini-code-assist[bot]@users.noreply.github.com"
	const devin = "158243242+devin-ai-integration[bot]@users.noreply.github.com"
	const codex = "267193182+codex@users.noreply.github.com"
	// name is the expected name when it differs from display.
	cases := []struct{ id, display, name, email string }{
		{"claude-opus-5-5", "Claude Opus 5.5", "", claude},
		{"anthropic/claude-opus-4.5", "Claude Opus 4.5 (latest)", "Claude Opus 4.5", claude},
		{"us.anthropic.claude-sonnet-4-5-v1:0", "", "us.anthropic.claude-sonnet-4-5-v1:0", claude},
		{"google/gemini-2.5-flash", "Gemini 2.5 Flash", "", gemini},
		{"gemini-4-neon-high", "Gemini 4 Neon", "", gemini},
		{"openai/gpt-5", "GPT-5", "", codex},
		{"gpt-5-5-high", "GPT-5.5", "", codex},
		{"o3-mini", "o3-mini", "", codex},
		{"codex-mini-latest", "Codex Mini", "", codex},
		{"swe-2-max", "SWE-2 Max", "", devin},
		{"swe-2-high-lightning", "SWE-2 High Lightning", "", devin},
		{"devin-2", "Devin 2", "", devin},
		{"fusion-gpt-6-sol-high-sidekick-claude-haiku-5-5-medium", "Fusion", "", codex},
		{"fusion-claude-opus-5-5-high-sidekick-swe-2-high", "Fusion", "", claude},
		{"fusion-swe-2-high-sidekick-gpt-6-sol-high", "Fusion", "", devin},
	}
	for _, c := range cases {
		name := cmp.Or(c.name, c.display)
		got, ok := authorFor(&ai.Model{ID: c.id, DisplayName: c.display})
		if want := name + " <" + c.email + ">"; !ok || got != want {
			t.Errorf("%s: identity = %q (%v), want %q", c.id, got, ok, want)
		}
	}
}

func TestAuthorForIsAbsentWithoutAVerifiedAccount(t *testing.T) {
	for _, id := range []string{"deepseek-v4", "glm-5", "qwen3-coder", "kimi-k2", "mistral-large", "grok-4", "llama-4", "gpt", "alpha", "o2", "pro-gemin", "glm-5-2", "kimi-k3-max", "nemotron-3-ultra-high", "fusion-glm-5-2-sidekick-claude-haiku-5-5-medium"} {
		if got, ok := authorFor(&ai.Model{ID: id, DisplayName: id}); ok {
			t.Errorf("%s: identity = %q, want none", id, got)
		}
	}
}

func TestAuthorNameCannotBreakTheIdentity(t *testing.T) {
	got, _ := authorFor(&ai.Model{ID: "claude-x", DisplayName: "Claude \"X\" <evil@example.com>\nit's"})
	if want := "Claude X evil@example.com its <noreply@anthropic.com>"; got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
}

func TestAuthorEnabledDefaultsOffAndHonoursTheSetting(t *testing.T) {
	cases := []struct {
		name     string
		settings extension.Settings
		want     bool
	}{
		{"absent", nil, false},
		{"empty object", extension.Settings{"modelAuthor": map[string]any{}}, false},
		{"explicit on", extension.Settings{"modelAuthor": map[string]any{"enabled": true}}, true},
		{"explicit off", extension.Settings{"modelAuthor": map[string]any{"enabled": false}}, false},
		{"wrong type", extension.Settings{"modelAuthor": map[string]any{"enabled": "yes"}}, false},
	}
	for _, c := range cases {
		if got := authorEnabled(c.settings); got != c.want {
			t.Errorf("%s: enabled = %v, want %v", c.name, got, c.want)
		}
	}
}

const (
	testIdentity = "Claude Opus 5.5 <noreply@anthropic.com>"
	flags        = " --author='Claude Opus 5.5 <noreply@anthropic.com>' --trailer='Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>'"
)

func TestRewriteCommits(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", `git commit -m "fix"`, "git commit" + flags + ` -m "fix"`},
		{"chained", `git add -A && git commit -m x && git push`, "git add -A && git commit" + flags + " -m x && git push"},
		{"semicolon and pipe", "git status; git commit -m x | cat", "git status; git commit" + flags + " -m x | cat"},
		{"global options", `git -C "my dir" -c user.name=x commit -m x`, `git -C "my dir" -c user.name=x commit` + flags + " -m x"},
		{"env assignment", "GIT_EDITOR=true git commit -m x", "GIT_EDITOR=true git commit" + flags + " -m x"},
		{"amend", "git commit --amend --no-edit", "git commit" + flags + " --amend --no-edit"},
		{"subshell and if", "(cd x && git commit -m a); if true; then git commit -m b; fi", "(cd x && git commit" + flags + " -m a); if true; then git commit" + flags + " -m b; fi"},
		{"line continuation", "git commit \\\n  -m x", "git commit" + flags + " \\\n  -m x"},
		{"heredoc message", "git commit -m \"$(cat <<'EOF'\nsay \"hi\"\ngit commit -m inner\nEOF\n)\"", "git commit" + flags + " -m \"$(cat <<'EOF'\nsay \"hi\"\ngit commit -m inner\nEOF\n)\""},
		{"heredoc stdin then next command", "git commit -F - <<EOF\ngit commit -m inner\nEOF\ngit commit -m next", "git commit" + flags + " -F - <<EOF\ngit commit -m inner\nEOF\ngit commit" + flags + " -m next"},
		{"indented heredoc", "git commit -F - <<-EOF\n\tgit commit x\n\tEOF\ngit log", "git commit" + flags + " -F - <<-EOF\n\tgit commit x\n\tEOF\ngit log"},
		{"nested substitution", `echo "$(git commit -m x)"`, `echo "$(git commit` + flags + ` -m x)"`},
		{"author already given", "git commit --author='A <a@b.c>' -m x", "git commit --trailer='Co-Authored-By: " + testIdentity + "' --author='A <a@b.c>' -m x"},
		{"trailer already given", `git commit --trailer "Co-Authored-By: ` + testIdentity + `" -m x`, "git commit --author='" + testIdentity + `' --trailer "Co-Authored-By: ` + testIdentity + `" -m x`},
	}
	for _, c := range cases {
		if got := rewriteCommits(c.in, testIdentity); got != c.want {
			t.Errorf("%s:\n got  %q\n want %q", c.name, got, c.want)
		}
	}
}

func TestRewriteCommitsLeavesLookalikesAlone(t *testing.T) {
	for name, in := range map[string]string{
		"quoted":        `echo "git commit -m x"`,
		"single quoted": "echo 'run git commit now'",
		"comment":       "ls # git commit -m x",
		"argument":      "grep -r 'git commit' . && echo git commit",
		"other git":     "git log --grep commit",
		"not git":       "mygit commit -m x",
		"heredoc only":  "cat <<EOF\ngit commit -m x\nEOF",
		"already both":  "git commit --author=A --trailer='Co-Authored-By: " + testIdentity + "' -m x",
	} {
		if got := rewriteCommits(in, testIdentity); got != in {
			t.Errorf("%s: rewrote %q to %q", name, in, got)
		}
	}
}

// benchCommands are a typical agent commit with a heredoc message, a long
// chained command, and a bash call that mentions commit but is not one.
var benchCommands = map[string]string{
	"heredoc commit": "git add -A && git commit -m \"$(cat <<'EOF'\nfeat: add the thing\n\n" + strings.Repeat("A paragraph of explanation that is fairly long and wordy.\n", 40) + "EOF\n)\" && git status",
	"chained":        strings.Repeat("echo step && ls -la | grep x; ", 40) + "git commit -m x",
	"lookalike":      "git log --grep commit --oneline | head -20",
}

func BenchmarkRewriteCommits(b *testing.B) {
	for name, command := range benchCommands {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				rewriteCommits(command, testIdentity)
			}
		})
	}
}

// TestRewrittenCommitIsAcceptedByGit runs the rewritten command in a real
// repository, so the flag spelling is checked against git itself.
func TestRewrittenCommitIsAcceptedByGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is required: %v", err)
	}
	dir := t.TempDir()
	script := strings.Join([]string{
		"git init -q",
		"git config user.name Human && git config user.email human@example.com",
		"echo a > f && git add f",
	}, " && ")
	original := "git commit -q -m \"$(cat <<'EOF'\nsubject\n\nbody with \"quotes\"\nEOF\n)\""
	command := rewriteCommits(original, "Gemini 3 Pro <176961590+gemini-code-assist[bot]@users.noreply.github.com>")
	if command == original {
		t.Fatal("command was not rewritten")
	}
	run := func(s string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "bash", "-c", s)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", s, err, out)
		}
		return string(out)
	}
	run(script + " && " + command)
	got := run("git log -1 --format='%an <%ae>%n%cn%n%B'")
	for _, want := range []string{
		"Gemini 3 Pro <176961590+gemini-code-assist[bot]@users.noreply.github.com>",
		"Human\n",
		"subject",
		"body with \"quotes\"",
		"Co-Authored-By: Gemini 3 Pro <176961590+gemini-code-assist[bot]@users.noreply.github.com>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("commit missing %q:\n%s", want, got)
		}
	}
}
