package codingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestExpandMultipleSkillCommands covers real resource reads, ordered deduplication,
// prose preservation, namespaced skills and retained invocation positions.
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
		if strings.Index(expanded, "Instructions for review") > strings.Index(expanded, "Instructions for team/check") || strings.Contains(expanded, "name: fixture") {
			t.Fatalf("bad expansion: %s", expanded)
		}
		if !strings.HasSuffix(expanded, "\n\n"+prompt) {
			t.Fatalf("source prompt changed: %s", expanded)
		}
		if _, again, err := ExpandSkillCommand(expanded, skills); again || err != nil {
			t.Fatal("already loaded skill expanded a second time")
		}
	}
	single, _, _ := ExpandSkillCommand("/skill:review hello", skills)
	original, _, _ := expandSingleSkillCommand("/skill:review", skills)
	if single != original+"\n\n/skill:review hello" {
		t.Fatal("single-skill source prompt was not retained")
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

// TestLegacySkillSubmissionRemainsOneBubble keeps old stored expansions
// readable without requiring the original skill files or source token positions.
func TestLegacySkillSubmissionRemainsOneBubble(t *testing.T) {
	for _, prose := range []string{"", "\n\nfix this"} {
		mode := resetWiringTest(t, nil)
		text := "<skill name=\"review\" location=\"/removed/review.md\">\nOLD BODY\n</skill>" + prose
		mode.addUserMessageToChat(agent.AgentMessage{}, text, messageMeta{})
		got := widthx.StripAnsi(strings.Join(mode.chatContainer.Render(80), "\n"))
		if len(mode.chatContainer.Children()) != 1 || !strings.Contains(got, "◆ review") || strings.Contains(got, "OLD BODY") {
			t.Fatalf("old expansion did not stay in one collapsed bubble: %q", got)
		}
		if prose != "" && !strings.Contains(got, "fix this") {
			t.Fatal("old expansion lost its trailing prompt")
		}
	}
}

// TestSkillExpansionRendersBadgesOnly drives the same entry point used live
// and on reload: skill badges stay inline in one submitted user bubble.
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
	expanded, ok, err := ExpandSkillCommand("please /skill:review fix /skill:test now", skills)
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
	if children := mode.chatContainer.Children(); len(children) != 1 {
		t.Fatalf("one submission split into %d transcript components", len(children))
	}
	for _, want := range []string{"please  ◆ review  fix  ◆ test  now"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("transcript missing %q: %q", want, transcript)
		}
	}
	// Resume from the persisted message, not from the current skill registry.
	// The original source and loaded bodies must be sufficient on their own.
	record, marshalErr := json.Marshal(map[string]any{
		"type": "message", "id": "saved-skill-prompt",
		"message": map[string]any{"role": "user", "content": []map[string]string{{"type": "text", "text": expanded}}},
	})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var entries []SessionEntry
	for _, record := range ParseSessionEntries(string(record)) {
		if entry, ok := record.(SessionEntry); ok {
			entries = append(entries, entry)
		}
	}
	mode.chatContainer.Clear() // rebuildChat clears before replaying entries.
	mode.renderSessionEntryList(entries, false)
	resumed := widthx.StripAnsi(strings.Join(mode.chatContainer.Render(90), "\n"))
	if resumed != transcript || len(mode.chatContainer.Children()) != 1 {
		t.Fatalf("resume changed the submitted bubble: %q; live %q", resumed, transcript)
	}
	for _, unwanted := range []string{"SECRET review BODY", "SECRET test BODY", "<skill name=", "</skill>"} {
		if strings.Contains(transcript, unwanted) {
			t.Fatalf("transcript printed %q: %q", unwanted, transcript)
		}
	}
}
