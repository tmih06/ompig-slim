package loopguard

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSessionDeterministicClasses(t *testing.T) {
	for _, test := range []struct {
		name, class, tool string
		args              func(int) string
	}{
		{"ambiguous edits", "edit-ambiguous", "edit", func(i int) string {
			return fmt.Sprintf(`{"path":"fixture.txt","edits":[{"oldText":"same result","newText":"replacement %d"}]}`, i)
		}},
		{"invalid regex", "grep-invalid-regex", "grep", func(i int) string {
			return fmt.Sprintf(`{"path":"fixture.txt","pattern":%q}`, strings.Repeat("[", i+1))
		}},
		{"nested varied edits", "edit-unmatched", "codemode", func(i int) string {
			code := fmt.Sprintf(`const results = await Promise.allSettled([tools.edit({path:"fixture.txt",edits:[{oldText:"absent %d",newText:"x"}]}),tools.bash({command:"printf ready"})]); text(results);`, i)
			args, err := json.Marshal(map[string]string{"code": code})
			if err != nil {
				t.Fatal(err)
			}
			return string(args)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var turns [][]fixtureCall
			for i := 0; i < 3; i++ {
				turns = append(turns, []fixtureCall{{test.tool, test.args(i)}})
			}
			requests, log, _ := runSessionFixture(t, turns)
			for i, request := range requests {
				if got := strings.Contains(request, "deterministic_tool_failure"); got != (i == 3) {
					t.Fatalf("request %d: deterministic steer present=%v, want %v\n%s", i+1, got, i == 3, log)
				}
			}
			if !strings.Contains(log, `"failureClass":"`+test.class+`"`) {
				t.Fatal("missing saved failure class", test.class)
			}
			if test.tool == "codemode" && (!strings.Contains(log, `"nestedCalls"`) || !strings.Contains(log, `"toolName":"edit","count":3`)) {
				t.Fatal("nested failure was not attributed to edit")
			}
		})
	}
}

func TestSessionExpectedNonmatches(t *testing.T) {
	var turns [][]fixtureCall
	for i := 0; i < 4; i++ {
		turns = append(turns, []fixtureCall{{"grep", fmt.Sprintf(`{"path":"fixture.txt","pattern":"absent %d"}`, i)}})
	}
	requests, log, _ := runSessionFixture(t, turns)
	for _, request := range requests {
		if strings.Contains(request, "deterministic_tool_failure") {
			t.Fatal("expected nonmatch triggered steer")
		}
	}
	if !strings.Contains(log, "No matches found") || strings.Contains(log, `"customType":"tool-call-loop-redirect"`) {
		t.Fatal("expected successful nonmatches without a steer")
	}
}
