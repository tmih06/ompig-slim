// Package pigidentity holds service identities. Most identify ompig-slim;
// Codex uses its native public-client identity for protocol compatibility.
//
// identity.json is the one source. automation/gen/pi-identity-patches.mjs
// applies its values to the Pi JavaScript that the Node extension runtime
// vendors, so a Node extension that calls the Pi SDK or pi-ai sends the identity
// the Go host sends. The Go host spells the same values as constants below, so a
// process decodes nothing at start; TestIdentityJSONIsTheSingleSource fails when
// identity.json and the constants differ.
package pigidentity

// pig divergence (D26): product values below replace Pi-branded literals;
// ompig-slim uses native Codex identity for that service alone. Other required
// client values (GitHub Copilot's editor headers, Anthropic OAuth's claude-cli)
// stay with their provider implementations.
const (
	// AttributionName replaces PI_ATTRIBUTION.name (`pi`): the OpenRouter app title, the NVIDIA invoke origin (first letter upper-cased) and the OpenCode client.
	AttributionName = "ompig-slim"
	// AttributionURL replaces PI_ATTRIBUTION.url (`https://pi.dev`): the OpenRouter `HTTP-Referer`.
	AttributionURL = "https://github.com/tmih06/omp-slim"
	// AttributionCategory is PI_ATTRIBUTION.categories' one entry, unchanged from Pi.
	AttributionCategory = "cli-agent"
	// CodexOriginator is the native Codex public-client identity, shared by the
	// browser authorization URL and HTTP/WebSocket API requests. Keep other
	// services on the product identity; this is a Codex protocol compatibility value.
	CodexOriginator = "codex_cli_rs"
	// ChatGPTAgentName replaces the `agent_name_hint=Pi` of the Sign in with ChatGPT authorization URL.
	ChatGPTAgentName = "ompig-slim"
	// XAIReferrer replaces the xAI device-code `referrer: pi`.
	XAIReferrer = "ompig-slim"
	// AgentMarker replaces the `AI_AGENT=pi` marker child processes inherit.
	AgentMarker = "ompig-slim"
	// UserAgentProduct replaces the `pi` product name in Pi's user agents.
	UserAgentProduct = "ompig-slim"
	// HostedOrigin is the PiG-operated origin that replaces Pi's pi.dev (D64).
	HostedOrigin = "https://pi-in-go.dev"
)
