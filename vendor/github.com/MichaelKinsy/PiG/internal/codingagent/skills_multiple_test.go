package codingagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
