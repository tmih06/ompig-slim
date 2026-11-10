package codingagent

import (
	"slices"
	"testing"

	"github.com/MichaelKinsy/PiG/tui"
)

// TestFollowUpDefaultIsCtrlQ pins the ompig-slim default for app.message.followUp: ctrl+q on every
// platform column, in place of upstream's alt+enter outside Windows and WSL. The binding table is the
// single source of truth — the startup header hint, `/hotkeys` and the `/settings` follow-up row all
// read it — so a default that drifted back to alt+enter fails here rather than only in a terminal.
func TestFollowUpDefaultIsCtrlQ(t *testing.T) {
	for _, platform := range tui.KeybindingPlatforms {
		definitions := appKeybindingDefinitionsFor(platform)
		definition, ok := definitions["app.message.followUp"]
		if !ok {
			t.Fatalf("%s: app.message.followUp is not defined", platform)
		}
		if want := []KeyID{"ctrl+q"}; !slices.Equal(definition.DefaultKeys, want) {
			t.Errorf("%s: app.message.followUp defaults to %q, want %q", platform, definition.DefaultKeys, want)
		}
	}
}

// TestFollowUpKeyMatcherCoversEveryTerminalProtocol checks that each input a terminal can send for ctrl+q
// reaches app.message.followUp through a manager with no user bindings: the legacy XON byte, the Kitty CSI-u
// form and the xterm modifyOtherKeys form. Raw mode clears IXON, so \x11 arrives as a keystroke instead of
// pausing output; the other two forms are what a terminal sends once it reports extended keys.
func TestFollowUpKeyMatcherCoversEveryTerminalProtocol(t *testing.T) {
	km := DefaultKeybindingsManager()
	for _, data := range []string{"\x11", "\x1b[113;5u", "\x1b[27;5;113~"} {
		if !km.Matches(data, "app.message.followUp") {
			t.Errorf("ctrl+q input %q does not queue a follow-up", data)
		}
	}
}

// TestFollowUpDefaultReleasesAltEnter records that alt+enter no longer reaches the follow-up action by
// default. Defining ctrl+q replaced the alt+enter default instead of adding to it, so the freed key is
// available to whatever a user or extension binds; on Apple Terminal, which cannot report Option+Return,
// ESC+Return now reaches the editor as a newline instead of queueing a follow-up.
func TestFollowUpDefaultReleasesAltEnter(t *testing.T) {
	km := DefaultKeybindingsManager()
	for _, data := range []string{"\x1b\r", "\x1b[13;3u", "\x1b[27;3;13~"} {
		if km.Matches(data, "app.message.followUp") {
			t.Errorf("alt+enter input %q still queues a follow-up", data)
		}
		if action := classifyKeyWithBindings(data, km); action == actionFollowUp {
			t.Errorf("alt+enter input %q still classifies as actionFollowUp", data)
		}
	}
}

// TestFollowUpDispatchReachesTheQueueAction pins the layer below the matcher: classifyKeyWithBindings must
// turn every ctrl+q input form into actionFollowUp, which resolveOutcome maps to outcomeFollowUp in both
// idle and working states. A binding that matched but classified as actionInsert would type nothing and
// queue nothing, so the two layers are checked together.
func TestFollowUpDispatchReachesTheQueueAction(t *testing.T) {
	km := DefaultKeybindingsManager()
	for _, data := range []string{"\x11", "\x1b[113;5u", "\x1b[27;5;113~"} {
		action := classifyKeyWithBindings(data, km)
		if action != actionFollowUp {
			t.Fatalf("ctrl+q input %q classifies as %v, want actionFollowUp", data, action)
		}
		for _, idle := range []bool{true, false} {
			if got := resolveOutcome(action, idle, false); got != outcomeFollowUp {
				t.Errorf("ctrl+q input %q with idle=%v resolves to %v, want outcomeFollowUp", data, idle, got)
			}
		}
	}
}
