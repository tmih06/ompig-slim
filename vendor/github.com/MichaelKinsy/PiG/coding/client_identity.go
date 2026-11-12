package coding

// Ports packages/coding-agent/src/core/client-identity.ts.

import (
	"github.com/MichaelKinsy/PiG/ai"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
)

// getClientIdentity identifies the client without optional provider attribution.
func getClientIdentity(settings *icodingagent.SettingsManager) *ai.ClientIdentity {
	return &ai.ClientIdentity{UserAgent: new(ai.UserAgentDetailed), Version: new(Version), AttributionNull: true}
}
