package streamingrules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinRules(t *testing.T) {
	data, err := os.ReadFile("testdata/builtin-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name    string   `json:"name"`
		Paths   []string `json:"paths"`
		Matches []string `json:"matches"`
		Misses  []string `json:"misses"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	rules, warnings := loadRules(t.TempDir(), t.TempDir(), readSettings(nil))
	if len(warnings) != 0 || len(rules) != len(fixtures) || len(rules) == 0 {
		t.Fatalf("rules=%d fixtures=%d warnings=%v", len(rules), len(fixtures), warnings)
	}
	byName := map[string]*rule{}
	for _, rule := range rules {
		byName[rule.name] = rule
	}
	seen := map[string]bool{}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			rule := byName[fixture.Name]
			if rule == nil || seen[fixture.Name] {
				t.Fatal("missing rule or duplicate fixture", fixture.Name)
			}
			seen[fixture.Name] = true
			if rule.interrupt || !rule.builtin || rule.summary == "" || len([]rune(rule.summary)) > 160 {
				t.Fatal("builtin must be advisory with a concise summary", rule)
			}
			if len(fixture.Paths) == 0 || len(fixture.Matches) < 2 || len(fixture.Misses) < 2 {
				t.Fatal("rule needs file scopes and positive and negative examples")
			}
			for _, file := range fixture.Paths {
				for _, want := range []bool{true, false} {
					examples := fixture.Matches
					if !want {
						examples = fixture.Misses
					}
					for i, text := range examples {
						t.Run(fmt.Sprintf("%s/%t/%d", file, want, i), func(t *testing.T) {
							for _, tool := range []string{"write", "edit"} {
								args := map[string]any{"path": file, "content": text, "edits": []any{map[string]any{"oldText": "old", "newText": text}}}
								matched := false
								for _, snapshot := range toolSnapshots(tool, args) {
									matched = matched || rule.matches(snapshot)
								}
								if matched != want {
									t.Fatalf("%s match=%v want=%v for %q", tool, matched, want, text)
								}
							}
						})
					}
				}
				if rule.matches(snapshot{tool: "read", path: file, text: fixture.Matches[0]}) {
					t.Fatal("matched read output")
				}
				for _, snapshot := range toolSnapshots("edit", map[string]any{"path": file, "edits": []any{map[string]any{"oldText": fixture.Matches[0], "newText": ""}}}) {
					if rule.matches(snapshot) {
						t.Fatal("matched deleted code")
					}
				}
			}
			for _, file := range []string{"example.md", "example.txt", fixture.Paths[0] + ".bak"} {
				if rule.matches(snapshot{tool: "write", path: file, text: fixture.Matches[0]}) {
					t.Fatal("matched a file outside its scope", file)
				}
			}
		})
	}
	for name := range byName {
		if !seen[name] {
			t.Error("untested bundled rule", name)
		}
	}
}

func TestAddedCodeOnly(t *testing.T) {
	for _, test := range []struct {
		tool string
		args map[string]any
		want []string
	}{
		{"edit", map[string]any{"path": "a.go", "edits": []any{
			map[string]any{"oldText": "bad", "newText": "good"},
			map[string]any{"oldText": "gone", "newText": ""},
		}}, []string{"good", ""}},
		{"edit", map[string]any{"path": "a.go", "oldText": "bad", "newText": "good"}, []string{"good"}},
		{"edit", map[string]any{"path": "a.go", "edits": []any{map[string]any{"oldText": "bad"}}}, nil},
		{"write", map[string]any{"path": "a.go", "content": "good"}, []string{"good"}},
		{"write", map[string]any{"path": "bad.go"}, nil},
		{"codemode", map[string]any{"code": "bad"}, nil},
	} {
		var got []string
		for _, snapshot := range toolSnapshots(test.tool, test.args) {
			if snapshot.path != "a.go" {
				t.Fatal(snapshot)
			}
			got = append(got, snapshot.text)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%s snapshots=%v want=%v", test.tool, got, test.want)
		}
	}
}

func TestRuleParsing(t *testing.T) {
	for _, header := range []string{
		"condition: '['", "condition: 42", "condition: [ok, 42]", "condition: ''",
		"condition: bad\nscope: text", "condition: bad\nscope: tool:write(**/*.go)",
		"condition: bad\ninterruptMode: prose-only", "condition: bad\nrepeatMode: always",
		"condition: bad\nastCondition: foo", "question: foo", "condition: bad\nglobs: '*.go'",
		"condition: bad\nagents: scout", "enabled: not-a-bool",
	} {
		if _, _, err := parseRule("custom", "custom.md", "---\n"+header+"\n---\nFix it.", false); err == nil {
			t.Errorf("accepted invalid header %q", header)
		}
	}
	for _, source := range []string{
		"---\ncondition: bad\n---\nFix it.",
		"---\nttsr_trigger: [bad, worse]\nscope: [tool:edit(*.go), tool:write(*.go)]\n---\nFix it.",
		"---\ncondition: '(?i)bad'\nscope: tool\ninterruptMode: never\n---\nFix it.",
	} {
		rule, disabled, err := parseRule("custom", "custom.md", source, false)
		if err != nil || disabled || rule == nil || !rule.matches(snapshot{tool: "write", path: "a.go", text: "bad"}) {
			t.Fatal(rule, disabled, err)
		}
	}
	if rule, disabled, err := parseRule("disabled", "disabled.md", "---\nenabled: false\n---\n", false); err != nil || !disabled || rule != nil {
		t.Fatal(rule, disabled, err)
	}
	if rule, _, err := parseRule("ordinary", "ordinary.md", "---\ndescription: ordinary rule\n---\nNot a streaming rule.", false); err != nil || rule != nil {
		t.Fatal(rule, err)
	}
}

func TestDiscovery(t *testing.T) {
	root, agent := t.TempDir(), t.TempDir()
	cwd := filepath.Join(root, "nested")
	write := func(file, data string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, ".git"), "worktree marker")
	write(filepath.Join(root, ".pi/rules/custom.md"), "---\ncondition: root\n---\nRoot.")
	write(filepath.Join(agent, "rules/custom.md"), "---\ncondition: global\n---\nGlobal.")
	write(filepath.Join(cwd, ".agents/rules/custom.mdc"), "---\ncondition: nested\n---\nNested.")
	write(filepath.Join(cwd, ".pi/rules/go-ioutil.md"), "---\nenabled: false\n---\n")
	write(filepath.Join(cwd, ".pi/rules/broken.md"), "---\ncondition: '['\n---\nBroken.")
	write(filepath.Join(cwd, ".pi/rules/ordinary.md"), "Ordinary instruction.")
	config := readSettings(map[string]any{"ttsr": map[string]any{"disabledRules": []any{"rs-box-leak"}}})
	rules, warnings := loadRules(cwd, agent, config)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "broken.md") {
		t.Fatal(warnings)
	}
	found := false
	for _, rule := range rules {
		if rule.name == "go-ioutil" || rule.name == "rs-box-leak" || rule.name == "ordinary" {
			t.Fatal("loaded disabled or ordinary rule", rule.name)
		}
		if rule.name == "custom" {
			found = true
			if rule.body != "Nested." || !rule.interrupt {
				t.Fatal("wrong precedence or default mode", rule)
			}
		}
	}
	if !found {
		t.Fatal("custom rule missing")
	}
	for _, config := range []settings{
		readSettings(map[string]any{"ttsr": map[string]any{"enabled": false}}),
		readSettings(map[string]any{"ttsr": map[string]any{"builtinRules": false}}),
	} {
		rules, warnings := loadRules(t.TempDir(), t.TempDir(), config)
		if len(rules) != 0 || len(warnings) != 0 {
			t.Fatal(rules, warnings)
		}
	}
}
