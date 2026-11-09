package coding

import (
	"reflect"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/extensions/sdk"
)

// TestRetiredModelScopeAPIsAreAbsent guards deletion rather than compatibility
// stubs: sessions and both extension surfaces expose only current model APIs.
func TestRetiredModelScopeAPIsAreAbsent(t *testing.T) {
	for _, value := range []any{(*Session)(nil), (*extension.Context)(nil), (*sdk.Context)(nil), (*sdk.ExtensionContext)(nil), (*SettingsManager)(nil), (*sdk.SettingsManager)(nil)} {
		typ := reflect.TypeOf(value)
		for _, name := range []string{"ScopedModels", "SetScopedModels", "GetEnabledModels", "SetEnabledModels"} {
			if _, exists := typ.MethodByName(name); exists {
				t.Errorf("retired %s.%s survives", typ, name)
			}
		}
	}
	for _, value := range []any{SessionOptions{}, SessionStartOptions{}, CreateAgentSessionOptions{}, CreateAgentSessionFromServicesOptions{}, extension.ContextActions{}, sdk.CreateAgentSessionOptions{}} {
		for _, name := range []string{"ScopedModels", "GetScopedModels"} {
			if _, exists := reflect.TypeOf(value).FieldByName(name); exists {
				t.Errorf("retired %T.%s option survives", value, name)
			}
		}
	}
}
