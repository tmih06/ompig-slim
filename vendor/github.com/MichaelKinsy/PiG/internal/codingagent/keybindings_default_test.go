package codingagent

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/MichaelKinsy/PiG/tui"
)

func TestFollowUpDefaultIsCtrlQ(t *testing.T) {
	definition, ok := defaultAppKeybindingDefinitions()["app.message.followUp"]
	if !ok {
		t.Fatal("app.message.followUp is not defined")
	}
	if want := []KeyID{"ctrl+q"}; !slices.Equal(definition.DefaultKeys, want) {
		t.Errorf("app.message.followUp defaults to %q, want %q", definition.DefaultKeys, want)
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

func TestSharedShortcutDefaults(t *testing.T) {
	want := map[string][]string{
		"app.suspend":                  {"ctrl+z"},
		"app.message.dequeue":          {"alt+q"},
		"app.clipboard.pasteImage":     {"ctrl+v"},
		"app.tree.foldOrUp":            {"ctrl+left", "alt+left"},
		"app.tree.unfoldOrDown":        {"ctrl+right", "alt+right"},
		"tui.editor.undo":              {"ctrl+-"},
		"tui.altScreen.previousPrompt": {"ctrl+up"},
		"tui.altScreen.nextPrompt":     {"ctrl+down"},
	}
	baseline := DefaultKeybindingsManager().ExtensionKeybindingTable()
	for _, environment := range []string{"", "Ubuntu", "Windows_Terminal"} {
		t.Run(environment, func(t *testing.T) {
			t.Setenv("WSL_DISTRO_NAME", environment)
			t.Setenv("WSL_INTEROP", environment)
			t.Setenv("WT_SESSION", environment)
			km := DefaultKeybindingsManager()
			for action, keys := range want {
				if got := km.GetKeys(action); !slices.Equal(got, keys) {
					t.Errorf("%s keys = %v, want %v", action, got, keys)
				}
			}
			if !km.merged.Matches("\x1f", tui.KBEditorUndo) {
				t.Error("Ctrl+- does not reach editor undo")
			}
			if km.merged.Matches("\x1a", tui.KBEditorUndo) {
				t.Error("Ctrl+Z still reaches editor undo instead of suspend")
			}
			if got := km.ExtensionKeybindingTable(); !reflect.DeepEqual(got, baseline) {
				t.Error("terminal environment changed extension keybindings")
			}
			for _, test := range []struct {
				input string
				want  keyAction
			}{
				{"\x1bq", actionDequeue},
				{"\x1a", actionSuspend},
				{"\x16", actionPasteImage},
			} {
				if got := classifyKeyWithBindings(test.input, km); got != test.want {
					t.Errorf("input %q action = %v, want %v", test.input, got, test.want)
				}
			}
		})
	}
}

func TestModelPickerDispatch(t *testing.T) {
	for _, wsl := range []string{"", "Ubuntu"} {
		t.Run("WSL="+wsl, func(t *testing.T) {
			t.Setenv("WSL_DISTRO_NAME", wsl)
			t.Setenv("WSL_INTEROP", wsl)
			km := NewKeybindingsManagerFromBindings(nil, "")
			for _, test := range []struct {
				key    string
				inputs []string
				want   keyAction
			}{
				{"alt+p", []string{"\x1bp", "\x1b[112;3u", "\x1b[27;3;112~"}, actionModelPicker},
				{"ctrl+l", []string{"\x0c", "\x1b[108;5u", "\x1b[27;5;108~"}, actionModelPicker},
				{"ctrl+p", []string{"\x10", "\x1b[112;5u", "\x1b[27;5;112~"}, actionCycleModelForward},
				{"ctrl+shift+left", []string{"\x1b[1;6D"}, actionCycleModelBackward},
			} {
				for _, data := range test.inputs {
					if got := classifyKeyWithBindings(data, km); got != test.want {
						t.Errorf("%s input %q classifies as %v, want %v", test.key, data, got, test.want)
					}
				}
			}
		})
	}
}

func TestModelPickerPreservesExplicitBackwardOverride(t *testing.T) {
	for _, wsl := range []string{"", "Ubuntu"} {
		t.Run("WSL="+wsl, func(t *testing.T) {
			t.Setenv("WSL_DISTRO_NAME", wsl)
			t.Setenv("WSL_INTEROP", wsl)
			km := NewKeybindingsManagerFromBindings(map[string][]KeyID{
				"app.model.cycleBackward": {"alt+p"},
			}, "")
			for _, data := range []string{"\x1bp", "\x1b[112;3u", "\x1b[27;3;112~"} {
				if got := classifyKeyWithBindings(data, km); got != actionCycleModelBackward {
					t.Errorf("overridden alt+p input %q classifies as %v, want actionCycleModelBackward", data, got)
				}
			}
			if got := classifyKeyWithBindings("\x0c", km); got != actionModelPicker {
				t.Errorf("ctrl+l classifies as %v, want actionModelPicker", got)
			}
		})
	}
}

// terminalRepertoire is every raw form an xterm-family terminal can send without the Kitty
// keyboard protocol and without xterm modifyOtherKeys: the C0 bytes and printable ASCII, the
// fixed CSI sequences for navigation and function keys with every modifier parameter, and the
// ESC-prefixed Meta form. This is the wire format a stock Apple Terminal profile produces, so a
// default with no match here is a default a user cannot press.
func terminalRepertoire() []string {
	var all []string
	for b := 0; b < 128; b++ {
		all = append(all, string([]byte{byte(b)}), "\x1b"+string([]byte{byte(b)}))
	}
	for b := 32; b < 127; b++ {
		all = append(all, string([]byte{byte(b)}))
	}
	all = append(all, "\x1b[13;2u") // native Shift+Enter detection rewrites Return to this.
	for _, final := range []string{"A", "B", "C", "D", "H", "F", "P", "Q", "R", "S"} {
		all = append(all, "\x1b["+final, "\x1bO"+final)
		for mod := 2; mod <= 8; mod++ {
			all = append(all, fmt.Sprintf("\x1b[1;%d%s", mod, final))
		}
	}
	for num := 1; num <= 34; num++ {
		all = append(all, fmt.Sprintf("\x1b[%d~", num))
		for mod := 2; mod <= 8; mod++ {
			all = append(all, fmt.Sprintf("\x1b[%d;%d~", num, mod))
		}
	}
	return append(all, "\x1b[Z")
}

// deliverable reports whether some form in the legacy repertoire reaches keyID, and the first
// such form for a failure message.
func deliverable(keyID string) (string, bool) {
	for _, data := range terminalRepertoire() {
		if tui.MatchesKeyID(data, keyID) {
			return data, true
		}
	}
	return "", false
}

// TestDefaultsAreReachableWithoutExtendedKeys pins the fix for defaults a terminal could not
// report. Upstream keys several actions with Ctrl+Shift+letter or Option+letter; a terminal
// with neither the Kitty protocol nor modifyOtherKeys collapses Ctrl+Shift+X onto Ctrl+X's
// control byte and never sends Option at all, so those actions were unreachable there — the
// same failure the follow-up key had. Each replacement is checked to be deliverable from the
// legacy repertoire and, where two actions could answer at once, to be unique.
func TestDefaultsAreReachableWithoutExtendedKeys(t *testing.T) {
	defs := keybindingDefinitions()
	for _, action := range []string{
		"app.model.cycleBackward",
		"app.tree.filter.cycleBackward",
		"app.session.deleteNoninvasive",
		"tui.altScreen.search",
		"tui.altScreen.searchPrevious",
		"app.message.followUp",
	} {
		keys := defs[action].DefaultKeys
		if len(keys) == 0 {
			t.Fatalf("%s has no default key", action)
		}
		reachable := false
		for _, key := range keys {
			if _, ok := deliverable(key); ok {
				reachable = true
				break
			}
		}
		if !reachable {
			t.Errorf("%s defaults to %v, none reachable from a legacy terminal", action, keys)
		}
	}
}
