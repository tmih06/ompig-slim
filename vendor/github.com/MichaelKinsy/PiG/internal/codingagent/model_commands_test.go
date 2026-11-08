package codingagent

import "testing"

// TestModelCommandRouting keeps persistent management on /model and session
// selection behind keyboard shortcuts; retired aliases never dispatch or list.
func TestModelCommandRouting(t *testing.T) {
	registry := NewSlashRegistry()
	hub := 0
	sc := &SlashContext{ShowModelHub: func() { hub++ }, Append: func(string) {}}
	for _, command := range []string{"/model"} {
		if err := registry.DispatchHeadless(sc, command, nil); err != nil {
			t.Fatal(err)
		}
	}
	if hub != 1 {
		t.Fatalf("manager=%d", hub)
	}
	for _, command := range []string{"/modelpreset", "/modelpreset save fixture", "/switch", "/switch @review", "/models", "/scoped-model", "/scoped-models", "/model @review"} {
		if _, ok := registry.Match(command); ok {
			t.Fatalf("retired command accepted: %s", command)
		}
	}
	for _, command := range BuiltinSlashCommands() {
		if command.Name == "modelpreset" || command.Name == "switch" || command.Name == "models" || command.Name == "scoped-model" || command.Name == "scoped-models" {
			t.Fatalf("retired command listed: %s", command.Name)
		}
	}
}
