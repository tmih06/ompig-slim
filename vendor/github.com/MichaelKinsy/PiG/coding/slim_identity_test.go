package coding

import (
	"testing"

	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
)

// TestSlimIdentityHasNoAttribution prevents legacy opt-ins from adding provider
// attribution and keeps the removed HTML API a local, explicit failure.
func TestSlimIdentityHasNoAttribution(t *testing.T) {
	t.Setenv("PI_TELEMETRY", "1")
	identity := getClientIdentity(icodingagent.NewInMemorySettingsManager(icodingagent.Settings{}))
	if identity.Attribution != nil || !identity.AttributionNull {
		t.Fatal("provider attribution enabled")
	}
}
