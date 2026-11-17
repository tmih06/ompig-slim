package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOMPMistralIdentityBoundary(t *testing.T) {
	for _, test := range []struct {
		name, body, id, email string
		status                int
	}{
		{"email", `{"id":"user","email":"user@example.invalid","workspace":{"id":"work"}}`, "user|workspace:work", "user@example.invalid", 200},
		{"no email", `{"id":"user","email":null}`, "user", "", 200},
		{"invalid email", `{"id":"user","email":42}`, "user", "", 200},
		{"missing id", `{"email":"user@example.invalid"}`, "", "", 200},
		{"invalid id", `{"id":42}`, "", "", 200},
		{"invalid JSON", `not json`, "", "", 200},
		{"unavailable", `{}`, "", "", 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			provider := mistralVibeOAuthProvider{client: server.Client(), apiBase: server.URL}
			credential := ompTestCredentialFromOAuth(t, provider.keyCredential(context.Background(), "mistral-secret"))
			wantID := test.id
			if wantID == "" {
				wantID = "key:" + hashSecret("mistral-secret")
			}
			if credential.AccountID != wantID || credentialClaim(credential, "email") != test.email {
				t.Fatalf("identity = %q, email = %q", credential.AccountID, credentialClaim(credential, "email"))
			}
			if strings.Contains(credential.AccountID, "mistral-secret") {
				t.Fatal("account identity exposes the API key")
			}
		})
	}
}

func TestOMPMistralIdentityDoesNotFollowRedirect(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("identity lookup followed a redirect")
	}))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer server.Close()
	provider := mistralVibeOAuthProvider{client: server.Client(), apiBase: server.URL}
	credential := provider.keyCredential(context.Background(), "mistral-secret")
	if credential.AccountID != "key:"+hashSecret("mistral-secret") {
		t.Fatal("redirect must retain the key-based fallback")
	}
}
