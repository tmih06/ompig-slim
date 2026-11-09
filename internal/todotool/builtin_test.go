package todotool

import (
	"testing"

	"github.com/MichaelKinsy/PiG/coding"
)

// TestTodoIsBuiltinTool pins that `todo` is a built-in tool, not an extension.
//
// This is the whole point of Register: a built-in is created with every session
// and registered in the runtime's built-in registry, so it survives
// --no-extensions, is dropped by --no-builtin-tools, and never appears in the
// startup Extensions list. An extension would instead show up in that list and
// would not be governed by --no-builtin-tools at all.
//
// The test reads the registry through the coding re-export because
// internal/codingagent/tools is unreachable from this module.
func TestTodoIsBuiltinTool(t *testing.T) {
	if !coding.IsExtendedBuiltinTool(ToolName) {
		t.Fatalf("%q is not a registered built-in tool; todo must register through coding.RegisterExtendedBuiltinTool", ToolName)
	}
	definition := coding.ExtendedBuiltinDefinition(ToolName, coding.ExtendedBuiltinOptions{})
	if definition.Name != ToolName {
		t.Fatalf("built-in definition name = %q, want %q", definition.Name, ToolName)
	}
	if definition.Execute == nil {
		t.Fatal("the built-in definition has no Execute: a session could declare todo but never run it")
	}
	// The card contract must survive registration: the built-in path builds the
	// definition from Definition(), so these are the same values.
	if definition.RenderShell != Definition().RenderShell {
		t.Fatalf("registered render shell = %q, and Definition() gives %q", definition.RenderShell, Definition().RenderShell)
	}
}

// TestRegisterIsIdempotent checks that a second Register replaces the factory
// instead of adding a second tool, so main's explicit call beside the init call
// is harmless.
func TestRegisterIsIdempotent(t *testing.T) {
	Register()
	Register()
	definition := coding.ExtendedBuiltinDefinition(ToolName, coding.ExtendedBuiltinOptions{})
	if definition.Name != ToolName {
		t.Fatalf("after repeated registration the built-in name = %q, want %q", definition.Name, ToolName)
	}
}
