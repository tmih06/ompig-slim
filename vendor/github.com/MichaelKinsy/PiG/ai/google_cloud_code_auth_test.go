package ai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// googleRefreshTransport models Google's binding of refresh grants to the
// issuing public desktop client. No credentials leave the test process.
type googleRefreshTransport struct {
	clientID, clientSecret string
}

// RoundTrip rejects a grant exchanged by the wrong desktop client, reproducing
// imported Antigravity accounts failing broker refresh with unauthorized_client.
func (transport googleRefreshTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.String() != googleCCATokenURL || request.Method != http.MethodPost {
		return nil, fmt.Errorf("unexpected refresh endpoint or method")
	}
	if err := request.ParseForm(); err != nil {
		return nil, err
	}
	status, body := http.StatusOK, `{"access_token":"fresh-access","expires_in":3600}`
	if request.Form.Get("client_id") != transport.clientID || request.Form.Get("client_secret") != transport.clientSecret {
		status, body = http.StatusUnauthorized, `{"error":"unauthorized_client"}`
	}
	if request.Form.Get("refresh_token") != "fixture-refresh" || request.Form.Get("grant_type") != "refresh_token" {
		return nil, fmt.Errorf("refresh lost grant")
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

// TestGoogleRefreshUsesIssuingClient exercises the registered OAuth providers,
// including the conversion and refresh path used by RefreshVaultCredential.
// Provider-specific desktop clients must remain distinct; project and identity
// metadata must survive renewal and no login/provisioning request is needed.
func TestGoogleRefreshUsesIssuingClient(t *testing.T) {
	for _, scenario := range []struct{ provider, clientID, clientSecret string }{
		{GoogleGeminiCliProviderID, googleCCAClientID, googleCCAClientSecret},
		{GoogleAntigravityProviderID, "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com", "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"},
	} {
		t.Run(scenario.provider, func(t *testing.T) {
			previous := http.DefaultClient
			http.DefaultClient = &http.Client{Transport: googleRefreshTransport{scenario.clientID, scenario.clientSecret}}
			t.Cleanup(func() { http.DefaultClient = previous })
			provider, ok := GetOAuthProvider(scenario.provider)
			if !ok {
				t.Fatal("missing registered OAuth provider")
			}
			fresh, err := oauthRefresh(provider)(context.Background(), Credential{Type: CredentialOAuth, Access: "expired-access", Refresh: "fixture-refresh", Expires: 1, ProjectID: "fixture-project"})
			if err != nil {
				t.Fatal(err)
			}
			if fresh.Access != "fresh-access" || fresh.Refresh != "fixture-refresh" || fresh.ProjectID != "fixture-project" || fresh.Expires <= 1 {
				t.Fatal("refresh lost token or account metadata")
			}
		})
	}
}
