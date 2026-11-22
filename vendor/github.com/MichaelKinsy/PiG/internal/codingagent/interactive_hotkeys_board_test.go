package codingagent

import (
	"io"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// /hotkeys lays its tables out for a board; the smoke test proves the
// transcript stays clear.
func TestHotkeysBoardListsTheShortcuts(t *testing.T) {
	m := &InteractiveMode{tuiInst: tui.NewWithOutput(io.Discard, 100, 24), editor: tui.NewEditor()}
	lines := m.hotkeysLines(76)
	text := widthx.StripTerminalSequences(strings.Join(lines, "\n"))
	for _, want := range []string{"Keyboard Shortcuts", "Navigation", "Send message", "Slash commands"} {
		if !strings.Contains(text, want) {
			t.Errorf("hotkeys board lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "**") || strings.Contains(text, "|-----") {
		t.Errorf("markdown was not rendered:\n%s", text)
	}
}
