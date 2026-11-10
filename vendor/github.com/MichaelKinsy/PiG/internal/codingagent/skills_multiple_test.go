package codingagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestExpandMultipleSkillCommands covers real resource reads, ordered deduplication,
// prose preservation, namespaced skills and the existing single-skill format.
func TestExpandMultipleSkillCommands(t *testing.T) {
	dir := t.TempDir()
	var skills []*SkillDef
	for _, name := range []string{"review", "team/check"} {
		path := filepath.Join(dir, strings.ReplaceAll(name, "/", "-")+".md")
		if err := os.WriteFile(path, []byte("---\nname: fixture\n---\nInstructions for "+name), 0600); err != nil {
			t.Fatal(err)
		}
		skills = append(skills, &SkillDef{Name: name, FilePath: path, BaseDir: dir})
	}
	for _, prompt := range []string{"/skill:review /skill:team/check fix it", "fix /skill:review this /skill:team/check now", "/skill:review /skill:review /skill:team/check"} {
		expanded, ok, err := ExpandSkillCommand(prompt, skills)
		if err != nil || !ok {
			t.Fatalf("%q: ok=%v err=%v", prompt, ok, err)
		}
		if strings.Count(expanded, "<skill name=\"review\"") != 1 || strings.Count(expanded, "<skill name=\"team/check\"") != 1 {
			t.Fatalf("missing or duplicate skill: %s", expanded)
		}
		if strings.Index(expanded, "Instructions for review") > strings.Index(expanded, "Instructions for team/check") || strings.Contains(expanded, "name: fixture") || strings.Contains(expanded, "/skill:") {
			t.Fatalf("bad expansion: %s", expanded)
		}
		if strings.HasPrefix(prompt, "fix ") && !strings.HasSuffix(expanded, "fix  this  now") {
			t.Fatalf("prose changed: %s", expanded)
		}
	}
	single, _, _ := ExpandSkillCommand("/skill:review hello", skills)
	original, _, _ := expandSingleSkillCommand("/skill:review hello", skills)
	if single != original {
		t.Fatal("single-skill format changed")
	}
	for _, prompt := range []string{"/model /skill:review", "!echo /skill:review", "$\necho /skill:review", "/skill:unknown"} {
		if _, ok, err := ExpandSkillCommand(prompt, skills); ok || err != nil {
			t.Fatalf("expanded literal input %q", prompt)
		}
	}
	expanded, ok, err := ExpandSkillCommand("/skill:review /skill:unknown", skills)
	if err != nil || !ok || !strings.HasSuffix(expanded, "/skill:unknown") {
		t.Fatal("unknown skill was consumed")
	}
	if err := os.Remove(skills[1].FilePath); err != nil {
		t.Fatal(err)
	}
	expanded, ok, err = ExpandSkillCommand("/skill:review /skill:team/check", skills)
	if err == nil || ok || expanded != "" {
		t.Fatal("unreadable second skill produced a partial expansion")
	}
}

// TestParseExpandedSkillMessage covers what the transcript relies on: every
// expanded block is recognized, so a second skill's XML and body never reach
// the user-message component as prose.
func TestParseExpandedSkillMessage(t *testing.T) {
	block := func(name, body string) string {
		return "<skill name=\"" + name + "\" location=\"/skills/" + name + "\">\nReferences are relative to /skills.\n\n" + body + "\n</skill>"
	}
	one := block("review", "REVIEW BODY")
	two := one + "\n\n" + block("test", "TEST BODY")

	cases := []struct {
		name   string
		text   string
		blocks []string
		user   string
	}{
		{"no skill", "just a prompt", nil, "just a prompt"},
		{"one block", one, []string{"review"}, ""},
		{"one block and text", one + "\n\nfix it", []string{"review"}, "fix it"},
		{"two blocks", two, []string{"review", "test"}, ""},
		{"two blocks and text", two + "\n\nfix it", []string{"review", "test"}, "fix it"},
		// A block that does not lead the message is not an expansion shape, so it
		// stays literal prose rather than silently disappearing.
		{"block after text", "hello " + one, nil, "hello " + one},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks, user := ParseExpandedSkillMessage(tc.text)
			var names []string
			for _, b := range blocks {
				names = append(names, b.Name)
			}
			if strings.Join(names, ",") != strings.Join(tc.blocks, ",") {
				t.Fatalf("blocks = %v, want %v", names, tc.blocks)
			}
			if user != tc.user {
				t.Fatalf("user = %q, want %q", user, tc.user)
			}
			// Recognized blocks must leave no markup in the user's text: that is
			// what the transcript would otherwise print as prose.
			if len(blocks) > 0 && (strings.Contains(user, "<skill") || strings.Contains(user, "</skill>")) {
				t.Fatalf("skill markup leaked into the user message: %q", user)
			}
		})
	}
}

// TestSkillExpansionRendersBadgesOnly drives the transcript: a prompt with two
// skills shows both badges and none of their bodies while collapsed.
func TestSkillExpansionRendersBadgesOnly(t *testing.T) {
	cwd := t.TempDir()
	var skills []*SkillDef
	for _, name := range []string{"review", "test"} {
		path := filepath.Join(cwd, name+".md")
		if err := os.WriteFile(path, []byte("---\nname: "+name+"\n---\nSECRET "+name+" BODY"), 0o600); err != nil {
			t.Fatal(err)
		}
		skills = append(skills, &SkillDef{Name: name, FilePath: path, BaseDir: cwd})
	}
	expanded, ok, err := ExpandSkillCommand("/skill:review /skill:test please fix", skills)
	if !ok || err != nil {
		t.Fatalf("expansion failed: ok=%v err=%v", ok, err)
	}
	mode := resetWiringTest(t, nil)
	mode.outputPad = 1
	mode.addUserMessageToChat(agent.AgentMessage{User: &agent.UserMessage{
		Role:    agent.RoleUser,
		Content: ai.UserContentBlocks{ai.TextContent{Text: expanded}},
	}}, expanded, messageMeta{})
	transcript := widthx.StripAnsi(strings.Join(mode.chatContainer.Render(90), "\n"))
	for _, want := range []string{"[skill] review", "[skill] test", "please fix"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("transcript missing %q: %q", want, transcript)
		}
	}
	for _, unwanted := range []string{"SECRET review BODY", "SECRET test BODY", "<skill name=", "</skill>"} {
		if strings.Contains(transcript, unwanted) {
			t.Fatalf("transcript printed %q: %q", unwanted, transcript)
		}
	}
}
