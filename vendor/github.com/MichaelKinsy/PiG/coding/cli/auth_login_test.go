package cli

import (
	"context"
	"io"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestAuthLoginBrowserPreferenceSelectsAlternativeWithoutPrompt(t *testing.T) {
	prompt := ai.AuthSelectPrompt{Message: "Sign-in method", Options: []ai.AuthSelectOption{{ID: "browser", Label: "Browser"}, {ID: "device", Label: "Device code"}}}
	for _, test := range []struct {
		name    string
		request authLoginRequest
		want    string
	}{{"default browser", authLoginRequest{provider: "fixture"}, "browser"}, {"no-browser alternative", authLoginRequest{provider: "fixture", noBrowser: true}, "device"}} {
		t.Run(test.name, func(t *testing.T) {
			interaction := authLoginInteraction(test.request, ai.CredentialOAuth, newAuthLoginTerminal(io.Discard))
			choice, err := interaction.Prompt(context.Background(), prompt)
			if err != nil || choice != test.want {
				t.Fatalf("choice=%q err=%v want=%q", choice, err, test.want)
			}
		})
	}
	// A browser-only provider must still work in manual-paste mode; no-browser
	// suppresses launch, not the provider's only authentication method.
	prompt.Options = prompt.Options[:1]
	interaction := authLoginInteraction(authLoginRequest{provider: "fixture", noBrowser: true}, ai.CredentialOAuth, newAuthLoginTerminal(io.Discard))
	if choice, err := interaction.Prompt(context.Background(), prompt); err != nil || choice != "browser" {
		t.Fatalf("browser-only choice=%q err=%v", choice, err)
	}
}

func TestAuthLoginTypePrefersAccountAndAllowsExplicitAPIKey(t *testing.T) {
	provider := &ai.ModelsProvider{Name: "Fixture", Auth: ai.ProviderAuth{OAuth: &ai.OAuthAuth{}, APIKey: &ai.APIKeyAuth{Login: func(context.Context, ai.AuthInteraction) (ai.Credential, error) { return ai.Credential{}, nil }}}}
	for _, test := range []struct {
		apiKey bool
		want   ai.AuthType
	}{{false, ai.CredentialOAuth}, {true, ai.CredentialAPIKey}} {
		if kind, err := selectAuthLoginType(provider, test.apiKey); err != nil || kind != test.want {
			t.Fatalf("apiKey=%v kind=%q err=%v", test.apiKey, kind, err)
		}
	}
	provider.Auth.OAuth = nil
	if kind, err := selectAuthLoginType(provider, false); err != nil || kind != ai.CredentialAPIKey {
		t.Fatalf("API-only provider kind=%q err=%v", kind, err)
	}
	provider.Auth.APIKey.Login = nil
	if _, err := selectAuthLoginType(provider, false); err == nil {
		t.Fatal("ambient-only credentials must not be presented as an interactive login")
	}
}
