package codingagent

// A skill invoked in a prompt is read at submission, exactly like an `@path`
// mention, so its transcript entry shows the same Read tree under the prompt:
//
//	◆ review  fix this
//	└─ Read .agents/skills/review/SKILL.md (34 lines)
//
// `@path` draws that row with fileMentionCard. A skill load had no equivalent:
// the body was inlined into the user message and the tree was missing, so the
// two loads looked different even though the same read had happened.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
)

// skillLoadFixture writes a skill under cwd and returns the definition that
// names it, so an expansion reads real content.
func skillLoadFixture(t *testing.T, cwd, name string) *SkillDef {
	t.Helper()
	dir := filepath.Join(cwd, ".agents", "skills", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	body := "---\nname: " + name + "\ndescription: Fixture skill\n---\n# Heading\n\nLine one.\nLine two.\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return &SkillDef{Name: name, Description: "Fixture skill", FilePath: path, BaseDir: dir, Body: body}
}

// submitSkillPrompt expands a prompt's skill tokens and renders the submission
// the way message_start does. It returns the sanitized transcript and the raw
// rows, because the indent must be read before whitespace is collapsed.
func submitSkillPrompt(t *testing.T, cwd, prompt string, skills ...*SkillDef) (sanitized string, raw []string) {
	t.Helper()
	mode := resetWiringTest(t, nil)
	mode.outputPad = 1
	mode.opts.CWD = cwd
	mode.opts.Skills = skills

	expanded, ok, err := ExpandSkillCommand(prompt, skills)
	if err != nil || !ok {
		t.Fatalf("expansion: ok=%v err=%v", ok, err)
	}
	mode.addUserMessageToChat(agent.AgentMessage{User: &agent.UserMessage{
		Role:    agent.RoleUser,
		Content: ai.UserContentBlocks{ai.TextContent{Text: expanded}},
	}}, expanded, messageMeta{})

	raw = mode.chatContainer.Render(90)
	rows := make([]string, len(raw))
	for i, row := range raw {
		rows[i] = tui.SanitizeDisplayLine(row)
	}
	return strings.Join(rows, "\n"), raw
}

// TestSkillLoadDrawsReadTreeUnderPrompt pins the transcript shape an `@path`
// mention already had: one Read row per loaded skill, under the prompt.
func TestSkillLoadDrawsReadTreeUnderPrompt(t *testing.T) {
	cwd := t.TempDir()
	skill := skillLoadFixture(t, cwd, "review")
	transcript, raw := submitSkillPrompt(t, cwd, "/skill:review fix this", skill)

	if !strings.Contains(transcript, "◆ review") {
		t.Fatalf("the skill badge is missing: %q", transcript)
	}
	if !strings.Contains(transcript, "└─ Read ") || !strings.Contains(transcript, ".agents/skills/review/SKILL.md") {
		t.Fatalf("the skill load drew no Read tree under the prompt: %q", transcript)
	}
	// The row belongs to the prompt above it, so it is indented with the prompt
	// block rather than drawn at the left edge.
	for _, row := range raw {
		if strings.Contains(tui.SanitizeDisplayLine(row), "└─ Read ") && !strings.HasPrefix(row, " ") {
			t.Fatalf("the Read row is not indented under the prompt: %q", row)
		}
	}
}

// TestSkillLoadReadTreeMatchesMentionShape keeps the two loads interchangeable:
// the same file read through either path renders the same row.
func TestSkillLoadReadTreeMatchesMentionShape(t *testing.T) {
	cwd := t.TempDir()
	skill := skillLoadFixture(t, cwd, "review")

	mentions := ReadFileMentions(t.Context(), []string{filepath.Join(".agents", "skills", "review", "SKILL.md")}, cwd)
	if len(mentions) != 1 {
		t.Fatalf("mention rows = %+v", mentions)
	}
	rows := skillReadRows([]ParsedSkillBlockFromText{{Name: "review", Location: skill.FilePath, Content: skill.Body}}, cwd)
	if len(rows) != 1 {
		t.Fatalf("skill rows = %+v", rows)
	}
	if got, want := rows[0].Path, mentions[0].Path; got != want {
		t.Fatalf("skill path %q != mention path %q", got, want)
	}
	if got, want := fileMentionDetail(rows[0]), fileMentionDetail(mentions[0]); got != want {
		t.Fatalf("skill detail %q != mention detail %q", got, want)
	}
}

// TestSkillLoadWithoutLocationDrawsNoRow covers an inline skill, whose
// invocation carries a location that is not a loadable Markdown file. There is
// nothing to report as read, so the row is omitted rather than invented.
func TestSkillLoadWithoutLocationDrawsNoRow(t *testing.T) {
	if rows := skillReadRows([]ParsedSkillBlockFromText{{Name: "inline", Content: "BODY"}}, t.TempDir()); len(rows) != 0 {
		t.Fatalf("an inline skill drew a read row: %+v", rows)
	}
}
