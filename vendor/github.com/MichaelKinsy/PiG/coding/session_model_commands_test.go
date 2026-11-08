package coding

import (
	"strings"
	"testing"
)

// TestModelCommandsDispatchArguments prevents new menu commands from being
// accidentally treated as paid model prompts when their argument suffix is used.
func TestModelCommandsDispatchArguments(t *testing.T) {
	s := roleTestSession(t, nil)
	for _, command := range []string{"/modelpreset save fixture", "/modelpreset list", "/modelpreset switch fixture", "/modelpreset delete fixture"} {
		out, err := s.DispatchSlash(command)
		if err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		if strings.HasPrefix(command, "/modelpreset") && len(out) == 0 {
			t.Fatalf("%s produced no result", command)
		}
	}
}
