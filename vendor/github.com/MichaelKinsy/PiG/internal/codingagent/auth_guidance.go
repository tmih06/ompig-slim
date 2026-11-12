package codingagent

import (
	"strings"

	"github.com/MichaelKinsy/PiG/internal/pigstrip"
)

const unknownProvider = "unknown"

// ProviderLoginHelp mirrors upstream auth-guidance.ts.
// pig additive (D92): a stripped /login or docs bundle leaves its sentence out.
func ProviderLoginHelp() string {
	var help []string
	if !pigstrip.Has(pigstrip.ListCommands, "/login") {
		help = append(help, "Use /login to log into a provider via OAuth or API key.")
	}
	return strings.Join(help, " ")
}

func FormatNoModelsAvailableMessage() string {
	return strings.TrimSuffix("No models available. "+ProviderLoginHelp(), " ")
}

func FormatNoModelSelectedMessage() string {
	parts := []string{"No model selected."}
	if help := ProviderLoginHelp(); help != "" {
		parts = append(parts, help)
	}
	// pig additive (D92): a stripped /model is not suggested.
	if !pigstrip.Has(pigstrip.ListCommands, "/model") {
		parts = append(parts, "Then use /model to select a model.")
	}
	return strings.Join(parts, "\n\n")
}

func FormatNoAPIKeyFoundMessage(provider string) string {
	providerDisplay := provider
	if providerDisplay == "" || providerDisplay == unknownProvider {
		providerDisplay = "the selected model"
	}
	return strings.TrimSuffix("No API key found for "+providerDisplay+".\n\n"+ProviderLoginHelp(), "\n\n")
}
