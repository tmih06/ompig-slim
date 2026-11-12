package codingagent

import (
	"reflect"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
)

// TestExtensionUIContextHasNoSpriteSurface keeps sprite customization out of the
// extension API. An extension must not be able to add a mascot, which would
// reintroduce the catalogue, preview overlay, and persisted selection that the
// slim build deliberately dropped.
func TestExtensionUIContextHasNoSpriteSurface(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"UIContext":    reflect.TypeOf((*extension.UIContext)(nil)).Elem(),
		"ExtUIContext": reflect.TypeOf((*ExtUIContext)(nil)),
	} {
		for i := 0; i < typ.NumMethod(); i++ {
			method := typ.Method(i).Name
			switch method {
			case "RegisterSprite", "UnregisterSprites":
				t.Fatalf("%s still exposes %s", name, method)
			}
		}
	}
}

// TestRemovedCommandsAreNotDispatched covers the registry itself, independent of
// the interactive rejection path, so no completion or headless dispatch offers a
// retired command.
func TestRemovedCommandsAreNotDispatched(t *testing.T) {
	registry := NewSlashRegistry()
	for _, command := range []string{"/sprite", "/pigsayhi", "/arminsayshi", "/dementedelves", "/share", "/changelog", "/llama"} {
		if _, ok := registry.Match(command); ok {
			t.Fatalf("%s is still dispatchable", command)
		}
	}
	if _, ok := registry.Match("/export"); !ok {
		t.Fatal("JSONL export was removed with HTML export")
	}
}
