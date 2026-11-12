package broker

import (
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestRemoteLogoutIdentitylessAccount preserves named and empty-key siblings
// across clients, including when a stale picker retries a removed NULL identity.
func TestRemoteLogoutIdentitylessAccount(t *testing.T) {
	a, b, _ := testClients(t)
	writer, reader := &remoteStore{client: a}, &remoteStore{client: b}
	ctx := t.Context()
	for _, key := range []*string{nil, new(""), new("keep")} {
		if _, err := writer.SaveAccount(ctx, "fixture", key, ai.Credential{Type: ai.CredentialAPIKey, Key: "fixture-key"}); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 2 {
		removed, err := reader.DeleteAccountIdentity(ctx, "fixture", nil)
		if err != nil || removed != (i == 0) {
			t.Fatalf("NULL removal %d: %v %v", i, removed, err)
		}
	}
	accounts, err := writer.ListAccounts(ctx)
	if err != nil || len(accounts) != 2 {
		t.Fatalf("remote siblings changed: %+v %v", accounts, err)
	}
	for _, account := range accounts {
		if account.AccountKey == nil {
			t.Fatal("NULL identity survived logout")
		}
	}
	if credential, err := writer.Read(ctx, "fixture"); err != nil || credential == nil || credential.Key != "fixture-key" {
		t.Fatalf("remaining account cannot authenticate: %+v %v", credential, err)
	}
}
