package codingagent

import "testing"

// TestModelCommandRouting keeps persistent management on /model and session
// selection behind keyboard shortcuts; retired aliases never dispatch or list.
func TestModelCommandRouting(t *testing.T) {
	registry := NewSlashRegistry()
	hub, picker := 0, 0
	sc := &SlashContext{ShowModelHub: func() { hub++ }, PickModel: func(string) (string, bool) { picker++; return "", false }, SwitchModel: func(string, func(error)) {}, Append: func(string) {}}
	for _, command := range []string{"/model"} {
		if err := registry.DispatchHeadless(sc, command, nil); err != nil {
			t.Fatal(err)
		}
	}
	if hub != 1 || picker != 0 {
		t.Fatalf("manager=%d picker=%d", hub, picker)
	}
	for _, command := range []string{"/switch", "/switch @review", "/models", "/scoped-model", "/scoped-models", "/model @review"} {
		if _, ok := registry.Match(command); ok {
			t.Fatalf("retired command accepted: %s", command)
		}
	}
	for _, command := range BuiltinSlashCommands() {
		if command.Name == "switch" || command.Name == "models" || command.Name == "scoped-model" || command.Name == "scoped-models" {
			t.Fatalf("retired command listed: %s", command.Name)
		}
	}
}
