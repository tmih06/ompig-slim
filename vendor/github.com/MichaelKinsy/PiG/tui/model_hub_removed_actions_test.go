package tui

import (
	"strings"
	"testing"
)

// TestModelHubHasNoPresetActions guards the complete feature removal, not only
// slash-command registration. Former keys cannot open strips or start mutations.
func TestModelHubHasNoPresetActions(t *testing.T) {
	h, _, _ := hubFixture()
	openRoles(h)
	h.sidebarFocus = false
	for _, key := range []string{"s", "p", "P", "\u001b[1;5D", "\u001b[1;5C"} {
		h.HandleInput(key)
		if h.strip != nil || h.pending {
			t.Fatalf("removed action handled %q", key)
		}
	}
	if strings.Contains(strings.ToLower(strings.Join(h.Render(140), "\n")), "preset") {
		t.Fatal("removed feature still advertised")
	}
}
