package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOMPMistralLoginPreservesLegacyAccount(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	legacy := ompTestCredentialFromOAuth(t, ompKeyCredential("legacy-mistral-key"))
	if _, err := store.Modify(ctx, "mistral", func(*Credential) (*Credential, error) { return &legacy, nil }); err != nil {
		t.Fatal(err)
	}
	next := ompKeyCredential("new-mistral-key")
	next.AccountID = "new-account"
	credential := ompTestCredentialFromOAuth(t, next)
	if _, err := store.Modify(ctx, "mistral", func(*Credential) (*Credential, error) { return &credential, nil }); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.ListAccounts(ctx)
	if err != nil || len(accounts) != 2 {
		t.Fatalf("accounts = %+v, err = %v; want legacy and new account", accounts, err)
	}
	preserved, err := store.ReadAccount(ctx, "mistral", nil)
	if err != nil || preserved == nil || preserved.Access != legacy.Access {
		t.Fatalf("legacy account was overwritten: %v", err)
	}
}

func TestOMPMistralLoginKeepsMultipleAccounts(t *testing.T) {
	for _, identityAvailable := range []bool{true, false} {
		t.Run(fmt.Sprint(identityAvailable), func(t *testing.T) {
			ctx := context.Background()
			store, _ := openTestStore(t)
			account, apiKey := "a", ""
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/vibe/sign-in":
					_ = json.NewEncoder(w).Encode(map[string]string{
						"process_id": "p", "sign_in_url": server.URL + "/console",
						"poll_url": server.URL + "/api/poll", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
					})
				case "/api/poll":
					_, _ = w.Write([]byte(`{"status":"completed","exchange_token":"exchange"}`))
				case "/api/vibe/sign-in/p/exchange":
					_ = json.NewEncoder(w).Encode(map[string]string{"api_key": apiKey})
				case "/v1/users/me":
					if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer "+apiKey {
						t.Error("identity request must use GET with the minted key")
					}
					if !identityAvailable {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id": account, "email": account + "@example.invalid",
						"workspace": map[string]string{"id": "workspace", "name": "Test"},
					})
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			provider := mistralVibeOAuthProvider{client: server.Client(), origin: server.URL, apiBase: server.URL + "/v1"}
			expectedKeys := map[string]string{}
			for attempt, next := range []string{"a", "b", "a"} {
				account = next
				apiKey = "mistral-secret-" + account
				if identityAvailable {
					apiKey += fmt.Sprint(attempt)
				}
				oauth, err := provider.LoginContext(ctx, OAuthLoginCallbacks{})
				if err != nil {
					t.Fatal(err)
				}
				credential := ompTestCredentialFromOAuth(t, oauth)
				expectedKeys[*deriveAccountKey(credential)] = apiKey
				if _, err := store.Modify(ctx, "mistral", func(*Credential) (*Credential, error) { return &credential, nil }); err != nil {
					t.Fatal(err)
				}
			}
			accounts, err := store.ListAccounts(ctx)
			if err != nil || len(accounts) != 2 {
				t.Fatalf("accounts = %+v, err = %v; want two accounts after a repeated login", accounts, err)
			}
			if identityAvailable && accounts[0].Label != "a@example.invalid" {
				t.Fatalf("active label = %q, want account email", accounts[0].Label)
			}
			for _, info := range accounts {
				if info.AccountKey == nil {
					t.Fatal("login must retain a selectable identity even when identity lookup fails")
				}
				if ok, err := store.SelectAccount(ctx, "mistral", *info.AccountKey); err != nil || !ok {
					t.Fatalf("select account: %v, %v", ok, err)
				}
				credential, err := store.Read(ctx, "mistral")
				if err != nil || credential == nil || credential.Access != expectedKeys[*info.AccountKey] {
					t.Fatalf("selected account did not resolve its latest key: %v", err)
				}
			}
		})
	}
}
