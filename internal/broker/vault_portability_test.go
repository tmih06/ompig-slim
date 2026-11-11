package broker

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// copyClosedAgentDB transfers only the main database, after its sole owner has
// closed SQLite and checkpointed the WAL. This deliberately exercises the file
// an operator backs up, not a logical export or a broker-only migration format.
func copyClosedAgentDB(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(ai.SQLiteAuthPath(source))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ai.SQLiteAuthPath(destination), data, 0600); err != nil {
		t.Fatal(err)
	}
}

// TestAgentDBTransfersBetweenBrokerAndStandalone pins the shared-file contract:
// the exact agent.db opens in either mode, carrying real tokens, identities,
// selection, logout tombstones and quota blocks without export/import conversion.
func TestAgentDBTransfersBetweenBrokerAndStandalone(t *testing.T) {
	previous := ai.CredentialStoreFactory
	ai.CredentialStoreFactory = nil
	t.Cleanup(func() { ai.CredentialStoreFactory = previous })
	ctx := context.Background()
	host, err := NewHandler(t.TempDir(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { host.Close() })
	for _, key := range []string{"one", "two", "retired"} {
		_, err := host.vault.SaveAccount(ctx, "fixture", &key, ai.Credential{
			Type: ai.CredentialOAuth, Access: key, Refresh: "private-" + key,
			Expires: time.Now().Add(time.Hour).UnixMilli(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := host.vault.SaveAccount(ctx, "api-fixture", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "private-api-key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := host.vault.DeleteAccount(ctx, "fixture", "retired"); err != nil {
		t.Fatal(err)
	}
	if _, err := host.vault.SelectAccount(ctx, "fixture", "two"); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Hour).UnixMilli()
	if err := host.vault.BlockAccount(ctx, "fixture", new("two"), "chat", until); err != nil {
		t.Fatal(err)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}

	standaloneDir := t.TempDir()
	copyClosedAgentDB(t, host.dir, standaloneDir)
	store, err := ai.OpenCredentialStore(standaloneDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	local, ok := store.(*ai.SQLiteAuthStorage)
	if !ok {
		t.Fatal("standalone discovery did not open the transferred SQLite vault")
	}
	accounts, err := local.ListAccounts(ctx)
	if err != nil || len(accounts) != 3 {
		t.Fatal("database transfer changed account records", err)
	}
	credential, err := store.Read(ctx, "fixture")
	if err != nil || credential == nil || credential.Access != "two" || credential.Refresh != "private-two" {
		t.Fatal("standalone mode lost the selected account or real refresh token", err)
	}
	blocked, err := store.GetAccountBlockedUntil(ctx, "fixture", new("two"), []string{"chat"})
	if err != nil || blocked == nil || *blocked != until {
		t.Fatal("database transfer lost quota blocks", err)
	}
	if _, err := store.ModifyAccount(ctx, "fixture", new("two"), func(current *ai.Credential) (*ai.Credential, error) {
		current.Access = "standalone-rotated"
		current.Refresh = "standalone-refresh"
		return current, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	restoredDir := t.TempDir()
	copyClosedAgentDB(t, standaloneDir, restoredDir)
	restored, err := NewHandler(restoredDir, "new-broker-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	credential, err = restored.vault.Read(ctx, "fixture")
	if err != nil || credential == nil || credential.Access != "standalone-rotated" || credential.Refresh != "standalone-refresh" {
		t.Fatal("broker could not reuse the standalone database", err)
	}
	if retired, err := restored.vault.ReadAccount(ctx, "fixture", new("retired")); err != nil || retired != nil {
		t.Fatal("database restore revived a logged-out account", err)
	}
	blocked, err = restored.vault.GetAccountBlockedUntil(ctx, "fixture", new("two"), []string{"chat"})
	if err != nil || blocked == nil || *blocked != until {
		t.Fatal("broker restore lost quota blocks", err)
	}
	entries, err := restored.entries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Credential.Type == ai.CredentialOAuth && entry.Credential.Refresh != remoteRefreshSentinel {
			t.Fatal("restored broker exposed a real refresh token")
		}
	}
}
