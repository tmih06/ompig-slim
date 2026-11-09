package todo

import (
	"testing"

	"github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
)

// TestRegisteredNatively pins that both halves of the feature are native.
//
// This is the contract the package exists to satisfy: `todo` is a built-in tool
// and its reminder is a native before-settle hook. Neither is an extension, so
// neither depends on the extension loader and neither can appear in the startup
// Extensions list. A regression that moved either half back onto the extension API
// would still work in a default run and fail only under --no-extensions, which is
// why this checks the registries directly.
func TestRegisteredNatively(t *testing.T) {
	if !tools.IsExtendedBuiltinTool(ToolName) {
		t.Fatalf("%q is not a registered built-in tool", ToolName)
	}
	definition := tools.ExtendedBuiltinDefinition(ToolName, tools.ExtendedBuiltinOptions{})
	if definition.Name != ToolName {
		t.Fatalf("built-in definition name = %q, want %q", definition.Name, ToolName)
	}
	if definition.Execute == nil {
		t.Fatal("the built-in definition has no Execute")
	}
	if !codingagent.HasNativeSettleHooks() {
		t.Fatal("no native settle hook registered: the reminder would never fire")
	}
	if n := len(codingagent.NativeSettleHooks()); n != 1 {
		t.Fatalf("registered %d settle hooks, want 1", n)
	}
}

// TestReminderPolicy drives the reminder's continue decision over the cases the
// live hook decides. The hook itself only reads the branch and delivers; the
// guards below are where a wrong continue would cost the user tokens, so each is
// pinned here rather than left to an end-to-end run that needs a live model.
func TestReminderPolicy(t *testing.T) {
	open := []Phase{{Name: "P", Tasks: []Task{{Content: "alpha", Status: StatusPending}, {Content: "beta", Status: StatusPending}}}}
	settled := []Phase{{Name: "P", Tasks: []Task{{Content: "alpha", Status: StatusCompleted}}}}
	blocked := []Phase{{Name: "P", Tasks: []Task{{Content: "alpha", Status: StatusBlocked}}}}

	t.Run("open work continues and delivers", func(t *testing.T) {
		reminder = reminderState{}
		var gotType, gotContent string
		if !reminder.remind(open, "", func(customType, content string) { gotType, gotContent = customType, content }) {
			t.Fatal("open work must continue the run")
		}
		if gotType != reminderCustomType || gotContent == "" {
			t.Fatalf("delivered (%q, %q), want the reminder custom type and content", gotType, gotContent)
		}
	})

	t.Run("an unchanged list is nudged once", func(t *testing.T) {
		reminder = reminderState{}
		deliver := func(string, string) {}
		if !reminder.remind(open, "", deliver) {
			t.Fatal("the first settle with open work must continue")
		}
		if reminder.remind(open, "", deliver) {
			t.Fatal("an unchanged list must not be nudged twice")
		}
	})

	t.Run("a question is a legitimate stop", func(t *testing.T) {
		reminder = reminderState{}
		if reminder.remind(open, "Done so far.\n\nShould I continue with beta?", func(string, string) {
			t.Fatal("a hand-off to the user must not be interrupted")
		}) {
			t.Fatal("a final question must be allowed to settle")
		}
	})

	t.Run("settled and blocked-only lists never nudge", func(t *testing.T) {
		reminder = reminderState{}
		deliver := func(string, string) { t.Fatal("nothing here is actionable") }
		if reminder.remind(settled, "", deliver) {
			t.Fatal("a finished list must not be nudged")
		}
		if reminder.remind(blocked, "", deliver) {
			t.Fatal("work waiting on someone else must not be nudged")
		}
	})

	t.Run("the budget is bounded", func(t *testing.T) {
		reminder = reminderState{}
		deliver := func(string, string) {}
		for i := 0; i < maxReminders; i++ {
			// Each list differs, so only the count can stop it.
			list := []Phase{{Name: "P", Tasks: []Task{{Content: string(rune('a' + i)), Status: StatusPending}}}}
			if !reminder.remind(list, "", deliver) {
				t.Fatalf("reminder %d should be allowed", i+1)
			}
		}
		if reminder.remind(open, "", deliver) {
			t.Fatalf("no reminder may exceed the budget of %d", maxReminders)
		}
	})
}
