package coding

import "testing"

// TestRemovedModelCommandsRejectHeadlessDispatch verifies removed model commands
// cannot mutate session model settings through the SDK slash boundary.
func TestRemovedModelCommandsRejectHeadlessDispatch(t *testing.T) {
	s := roleTestSession(t, nil)
	for _, command := range []string{"/modelpreset", "/modelpreset save fixture", "/switch @review", "/models", "/scoped-models"} {
		if _, err := s.DispatchSlash(command); err == nil {
			t.Fatalf("retired command accepted: %s", command)
		}
	}
}
