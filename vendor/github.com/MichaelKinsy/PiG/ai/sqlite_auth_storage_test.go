package ai

// Behavioral tests for the multi-account SQLite credential store and the one-time auth.json
// migration. They exercise OMP's schema through the store's public surface: identity-scoped
// tombstones, active selection, block scopes, and the cross-process refresh lease.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/extensions/sdk/json"
)

func openTestStore(t *testing.T) (*SQLiteAuthStorage, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := OpenSQLiteAuthStorage(SQLiteAuthPath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, dir
}

func oauthCredential(accountID, email string, expires time.Time) Credential {
	return Credential{
		Type:      CredentialOAuth,
		Access:    "access-" + accountID,
		Refresh:   "refresh-" + accountID,
		AccountID: accountID,
		Expires:   expires.UnixMilli(),
		Extra:     map[string]json.RawMessage{"email": jsonRaw(email)},
	}
}

func TestSQLiteStoreSavesSeveralAccountsPerProvider(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	future := time.Now().Add(time.Hour)

	first := oauthCredential("acct-a", "a@example.com", future)
	if _, err := store.SaveAccount(ctx, "openai-codex", deriveAccountKey(first), first); err != nil {
		t.Fatalf("save first: %v", err)
	}
	second := oauthCredential("acct-b", "b@example.com", future)
	accounts, err := store.SaveAccount(ctx, "openai-codex", deriveAccountKey(second), second)
	if err != nil {
		t.Fatalf("save second: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("want 2 accounts, got %d: %+v", len(accounts), accounts)
	}
	// Saving an account selects it, as "log in and use it" does.
	active := ""
	for _, account := range accounts {
		if account.Active {
			if account.AccountKey == nil {
				t.Fatal("active account has no key")
			}
			active = *account.AccountKey
		}
	}
	if active != "acct-b" {
		t.Fatalf("active account = %q, want acct-b", active)
	}
	read, err := store.Read(ctx, "openai-codex")
	if err != nil || read == nil || read.AccountID != "acct-b" {
		t.Fatalf("Read = %+v, %v; want the selected acct-b", read, err)
	}
	// Select the first account back and read it by key.
	selected, err := store.SelectAccount(ctx, "openai-codex", "acct-a")
	if err != nil || !selected {
		t.Fatalf("select acct-a = %v, %v", selected, err)
	}
	read, err = store.Read(ctx, "openai-codex")
	if err != nil || read == nil || read.AccountID != "acct-a" {
		t.Fatalf("Read after select = %+v, %v; want acct-a", read, err)
	}
	byKey, err := store.ReadAccount(ctx, "openai-codex", new("acct-b"))
	if err != nil || byKey == nil || byKey.AccountID != "acct-b" {
		t.Fatalf("ReadAccount(acct-b) = %+v, %v", byKey, err)
	}
}

func TestSQLiteStoreDeleteAccountKeepsSiblings(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	future := time.Now().Add(time.Hour)
	first := oauthCredential("acct-a", "a@example.com", future)
	second := oauthCredential("acct-b", "b@example.com", future)
	if _, err := store.SaveAccount(ctx, "anthropic", deriveAccountKey(first), first); err != nil {
		t.Fatalf("save first: %v", err)
	}
	if _, err := store.SaveAccount(ctx, "anthropic", deriveAccountKey(second), second); err != nil {
		t.Fatalf("save second: %v", err)
	}
	removed, err := store.DeleteAccount(ctx, "anthropic", "acct-b")
	if err != nil || !removed {
		t.Fatalf("DeleteAccount = %v, %v", removed, err)
	}
	accounts, err := store.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].AccountKey == nil || *accounts[0].AccountKey != "acct-a" {
		t.Fatalf("after delete accounts = %+v", accounts)
	}
	if !accounts[0].Active {
		// Deleting the selected account clears its selection, so the remaining one becomes active.
		t.Fatal("remaining account should be active after the selected one was removed")
	}
	// A tombstone, not a DELETE: the row survives with disabled_cause = logout.
	if _, exists, err := rowIDByAccountKey(ctx, store.db, "anthropic", new("acct-b")); err != nil || !exists {
		t.Fatalf("tombstoned row missing: exists=%v err=%v", exists, err)
	}
	if stillThere, err := store.ReadAccount(ctx, "anthropic", new("acct-b")); err != nil || stillThere != nil {
		t.Fatalf("ReadAccount(tombstoned) = %+v, %v; want nil", stillThere, err)
	}
}

func TestSQLiteStoreRevivesTombstoneOnLogin(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	future := time.Now().Add(time.Hour)
	credential := oauthCredential("acct-a", "a@example.com", future)
	key := deriveAccountKey(credential)
	if _, err := store.SaveAccount(ctx, "anthropic", key, credential); err != nil {
		t.Fatalf("save: %v", err)
	}
	if removed, err := store.DeleteAccount(ctx, "anthropic", *key); err != nil || !removed {
		t.Fatalf("delete = %v, %v", removed, err)
	}
	if _, err := store.SaveAccount(ctx, "anthropic", key, credential); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	accounts, err := store.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("a revived login must reuse the tombstoned row, got %d rows", len(accounts))
	}
}

func TestSQLiteStoreIdentityLessRowIsNotTheActiveRow(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	future := time.Now().Add(time.Hour)
	named := oauthCredential("acct-a", "a@example.com", future)
	if _, err := store.SaveAccount(ctx, "openai-codex", deriveAccountKey(named), named); err != nil {
		t.Fatalf("save named: %v", err)
	}
	anonymous := Credential{Type: CredentialAPIKey, Key: "sk-identity-less"}
	if _, err := store.SaveAccount(ctx, "openai-codex", nil, anonymous); err != nil {
		t.Fatalf("save anonymous: %v", err)
	}
	// The identity-less write selected itself, so Read resolves it.
	active, err := store.Read(ctx, "openai-codex")
	if err != nil || active == nil || active.Key != anonymous.Key {
		t.Fatalf("Read = %+v, %v; want the identity-less key", active, err)
	}
	// ReadAccount(nil) names the identity-less row explicitly; a named key never falls back to it.
	byNilKey, err := store.ReadAccount(ctx, "openai-codex", nil)
	if err != nil || byNilKey == nil || byNilKey.Key != anonymous.Key {
		t.Fatalf("ReadAccount(nil) = %+v, %v", byNilKey, err)
	}
	byNamedKey, err := store.ReadAccount(ctx, "openai-codex", new("acct-a"))
	if err != nil || byNamedKey == nil || byNamedKey.AccountID != "acct-a" {
		t.Fatalf("ReadAccount(acct-a) = %+v, %v", byNamedKey, err)
	}
	// Selecting the named account leaves the identity-less row addressable by its own key.
	if selected, err := store.SelectAccount(ctx, "openai-codex", "acct-a"); err != nil || !selected {
		t.Fatalf("select named = %v, %v", selected, err)
	}
	if again, err := store.ReadAccount(ctx, "openai-codex", nil); err != nil || again == nil || again.Key != anonymous.Key {
		t.Fatalf("ReadAccount(nil) after selecting a sibling = %+v, %v", again, err)
	}
}

func TestSQLiteStoreOAuthLoginReplacesAPIKeys(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := store.SaveAccount(ctx, "github-copilot", nil, Credential{Type: CredentialAPIKey, Key: "sk-old"}); err != nil {
		t.Fatalf("save key: %v", err)
	}
	oauth := oauthCredential("acct-a", "a@example.com", time.Now().Add(time.Hour))
	if _, err := store.SaveAccount(ctx, "github-copilot", deriveAccountKey(oauth), oauth); err != nil {
		t.Fatalf("save oauth: %v", err)
	}
	accounts, err := store.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Type != CredentialOAuth {
		t.Fatalf("an OAuth login must retire the provider's API keys, got %+v", accounts)
	}
}

func TestSQLiteStoreBlockScopesAndLongestDeadline(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	credential := oauthCredential("acct-a", "a@example.com", time.Now().Add(time.Hour))
	key := deriveAccountKey(credential)
	if _, err := store.SaveAccount(ctx, "openai-codex", key, credential); err != nil {
		t.Fatalf("save: %v", err)
	}
	now := time.Now().UnixMilli()
	if err := store.BlockAccount(ctx, "openai-codex", key, "chat", now+60_000); err != nil {
		t.Fatalf("block chat: %v", err)
	}
	if err := store.BlockAccount(ctx, "openai-codex", key, "spark", now+120_000); err != nil {
		t.Fatalf("block spark: %v", err)
	}
	// A shorter write never shortens a live deadline.
	if err := store.BlockAccount(ctx, "openai-codex", key, "chat", now+1_000); err != nil {
		t.Fatalf("re-block chat: %v", err)
	}
	until, err := store.GetAccountBlockedUntil(ctx, "openai-codex", key, []string{"chat", "spark"})
	if err != nil || until == nil {
		t.Fatalf("blocked until = %v, %v", until, err)
	}
	if *until != now+120_000 {
		t.Fatalf("blocked until = %d, want the longest deadline %d", *until, now+120_000)
	}
	// The unscoped row applies to every scope.
	if err := store.BlockAccount(ctx, "openai-codex", key, "", now+300_000); err != nil {
		t.Fatalf("block unscoped: %v", err)
	}
	until, err = store.GetAccountBlockedUntil(ctx, "openai-codex", key, []string{"auth"})
	if err != nil || until == nil || *until != now+300_000 {
		t.Fatalf("unscoped block = %v, %v; want %d", until, err, now+300_000)
	}
	if none, err := store.GetAccountBlockedUntil(ctx, "openai-codex", nil, []string{"chat"}); err != nil || none != nil {
		t.Fatalf("identity-less account has no blocks: %v, %v", none, err)
	}
}

func TestSQLiteStoreModifyAccountLeasesOAuthRefresh(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	credential := oauthCredential("acct-a", "a@example.com", time.Now().Add(-time.Minute))
	key := deriveAccountKey(credential)
	if _, err := store.SaveAccount(ctx, "anthropic", key, credential); err != nil {
		t.Fatalf("save: %v", err)
	}
	// While another owner holds the row's lease, a refresh must not spend the token: it waits out
	// its budget and reports the stored credential unchanged.
	if _, err := store.db.ExecContext(ctx, `INSERT INTO auth_credential_refresh_leases
		(credential_id, owner, expires_at_ms, updated_at) VALUES (1, 'peer', ?, 0)`,
		time.Now().Add(time.Minute).UnixMilli()); err != nil {
		t.Fatalf("hold lease: %v", err)
	}
	started := time.Now()
	_, err := store.Modify(ctx, "anthropic", func(current *Credential) (*Credential, error) {
		t.Fatal("callback ran while a peer held the refresh lease")
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "lease timeout") {
		t.Fatalf("modify under a peer lease = %v; want a lease timeout", err)
	}
	if elapsed := time.Since(started); elapsed < 100*time.Millisecond {
		t.Fatalf("gave up after %s without waiting for the holder", elapsed)
	}
	// A peer that rotated the row meanwhile is the truth: the refresh reports its credential.
	if _, err := store.db.ExecContext(ctx, `DELETE FROM auth_credential_refresh_leases WHERE credential_id = 1`); err != nil {
		t.Fatalf("release lease: %v", err)
	}
	rotated, err := store.ModifyAccount(ctx, "anthropic", key, func(current *Credential) (*Credential, error) {
		if current == nil {
			t.Fatal("refresh saw no credential")
		}
		next := *current
		next.Access = "rotated"
		next.Refresh = "rotated"
		next.Expires = time.Now().Add(time.Hour).UnixMilli()
		return &next, nil
	})
	if err != nil || rotated == nil || rotated.Access != "rotated" {
		t.Fatalf("refresh = %+v, %v", rotated, err)
	}
	// The lease is released after the refresh, so a sibling can take it next time.
	var leases int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM auth_credential_refresh_leases").Scan(&leases); err != nil {
		t.Fatalf("count leases: %v", err)
	}
	if leases != 0 {
		t.Fatalf("refresh left %d lease rows behind", leases)
	}
}

func TestSQLiteStoreModifyDoesNotResurrectLoggedOutAccount(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	credential := oauthCredential("acct-a", "a@example.com", time.Now().Add(time.Hour))
	key := deriveAccountKey(credential)
	if _, err := store.SaveAccount(ctx, "anthropic", key, credential); err != nil {
		t.Fatalf("save: %v", err)
	}
	if removed, err := store.DeleteAccount(ctx, "anthropic", *key); err != nil || !removed {
		t.Fatalf("delete = %v, %v", removed, err)
	}
	called := false
	result, err := store.ModifyAccount(ctx, "anthropic", key, func(current *Credential) (*Credential, error) {
		called = true
		return current, nil
	})
	if err != nil {
		t.Fatalf("modify tombstoned account: %v", err)
	}
	if called || result != nil {
		t.Fatalf("a refresh of a logged-out account must not run or write: called=%v result=%+v", called, result)
	}
}

func TestSQLiteStoreReadOnlyRejectsWrites(t *testing.T) {
	store, dir := openTestStore(t)
	ctx := context.Background()
	credential := oauthCredential("acct-a", "a@example.com", time.Now().Add(time.Hour))
	if _, err := store.SaveAccount(ctx, "anthropic", deriveAccountKey(credential), credential); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	readOnly, err := OpenReadOnlySQLiteAuthStorage(SQLiteAuthPath(dir))
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer func() { _ = readOnly.Close() }()
	read, err := readOnly.Read(ctx, "anthropic")
	if err != nil || read == nil {
		t.Fatalf("read-only read = %+v, %v", read, err)
	}
	if _, err := readOnly.SaveAccount(ctx, "anthropic", nil, Credential{Type: CredentialAPIKey, Key: "x"}); err == nil {
		t.Fatal("read-only store accepted a write")
	}
	if _, err := readOnly.Modify(ctx, "anthropic", func(*Credential) (*Credential, error) { return nil, nil }); err == nil {
		t.Fatal("read-only store accepted a modify")
	}
	if err := readOnly.Delete(ctx, "anthropic"); err == nil {
		t.Fatal("read-only store accepted a delete")
	}
}

func TestSQLiteStoreFileIsOwnerOnly(t *testing.T) {
	store, dir := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	info, err := os.Stat(SQLiteAuthPath(dir))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("database mode = %#o, want 0600", mode)
	}
}

func TestMigrateAuthJSONImportsCredentialAndRenamesFile(t *testing.T) {
	store, dir := openTestStore(t)
	authPath := filepath.Join(dir, "auth.json")
	contents := `{"anthropic": {"type": "oauth", "refresh": "r", "access": "a", "expires": 4102444800000, ` +
		`"accountId": "acct-a"}, "openai": {"type": "api_key", "key": "sk-live"}}`
	if err := os.WriteFile(authPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	result := MigrateAuthJSONToSQLite(store, dir)
	if len(result.MigratedProviders) != 2 {
		t.Fatalf("migrated providers = %v", result.MigratedProviders)
	}
	if result.ArchivedPath != authPath+".migrated" {
		t.Fatalf("archived path = %q", result.ArchivedPath)
	}
	if _, err := os.Stat(authPath); !os.IsNotExist(err) {
		t.Fatalf("auth.json still present after migration: %v", err)
	}
	if _, err := os.Stat(result.ArchivedPath); err != nil {
		t.Fatalf("archived file missing: %v", err)
	}
	accounts, err := store.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("migrated accounts = %+v", accounts)
	}
	for _, account := range accounts {
		if !account.Active {
			t.Fatalf("each migrated provider's only account must be active: %+v", account)
		}
	}
	// A second migration is a no-op: no legacy file remains, and a non-empty store is authoritative.
	if again := MigrateAuthJSONToSQLite(store, dir); len(again.MigratedProviders) != 0 {
		t.Fatalf("second migration imported %v", again.MigratedProviders)
	}
}

func TestMigrateAuthJSONSkipsNonEmptyStore(t *testing.T) {
	store, dir := openTestStore(t)
	ctx := context.Background()
	existing := oauthCredential("acct-keep", "keep@example.com", time.Now().Add(time.Hour))
	if _, err := store.SaveAccount(ctx, "anthropic", deriveAccountKey(existing), existing); err != nil {
		t.Fatalf("save: %v", err)
	}
	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"openai": {"type": "api_key", "key": "sk-live"}}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if result := MigrateAuthJSONToSQLite(store, dir); len(result.MigratedProviders) != 0 {
		t.Fatalf("migrated into a non-empty store: %v", result.MigratedProviders)
	}
	if _, err := os.Stat(authPath); err != nil {
		t.Fatalf("legacy file must be left in place: %v", err)
	}
	accounts, err := store.ListAccounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].AccountKey == nil || *accounts[0].AccountKey != "acct-keep" {
		t.Fatalf("store contents changed: %+v, %v", accounts, err)
	}
}

func TestMigrateAuthJSONIgnoresMalformedFile(t *testing.T) {
	store, dir := openTestStore(t)
	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"anthropic": `), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if result := MigrateAuthJSONToSQLite(store, dir); len(result.MigratedProviders) != 0 {
		t.Fatalf("imported from a malformed file: %v", result.MigratedProviders)
	}
	if _, err := os.Stat(authPath); err != nil {
		t.Fatalf("malformed file must stay for inspection: %v", err)
	}
}

func TestDeriveAccountKeyIdentityRules(t *testing.T) {
	oauth := Credential{Type: CredentialOAuth, AccountID: "acct", ProjectID: "proj"}
	oauth.Extra = map[string]json.RawMessage{"email": jsonRaw("a@example.com"), "orgId": jsonRaw("org-1")}
	if key := deriveAccountKey(oauth); key == nil || *key != "acct|org:org-1" {
		t.Fatalf("oauth key = %v, want acct|org:org-1", key)
	}
	emailOnly := Credential{Type: CredentialOAuth}
	emailOnly.Extra = map[string]json.RawMessage{"email": jsonRaw("a@example.com")}
	if key := deriveAccountKey(emailOnly); key == nil || *key != "a@example.com" {
		t.Fatalf("email key = %v", key)
	}
	noClaims := Credential{Type: CredentialOAuth, Access: "a", Refresh: "r"}
	if key := deriveAccountKey(noClaims); key != nil {
		t.Fatalf("identity-less oauth key = %v, want nil", key)
	}
	envKey := Credential{Type: CredentialAPIKey, Key: "sk-live", Env: map[string]string{"OPENAI_ORG_ID": "org-9"}}
	if key := deriveAccountKey(envKey); key == nil || *key != "env:org-9" {
		t.Fatalf("env-hinted key = %v", key)
	}
	shortKey := Credential{Type: CredentialAPIKey, Key: "sk-short"}
	if key := deriveAccountKey(shortKey); key == nil || *key != "key:sk-short" {
		t.Fatalf("short key = %v", key)
	}
	longKey := Credential{Type: CredentialAPIKey, Key: "sk-a-long-secret-value"}
	key := deriveAccountKey(longKey)
	if key == nil || !strings.HasPrefix(*key, "key:") || strings.Contains(*key, "secret") {
		t.Fatalf("a long key must be hashed, got %v", key)
	}
	if len(*key) != len("key:")+16 {
		t.Fatalf("hashed key = %q, want 16 hex digits", *key)
	}
	// The digest is stable for the same secret and different for another one.
	if again := deriveAccountKey(longKey); again == nil || *again != *key {
		t.Fatal("hashed key is not deterministic")
	}
	other := Credential{Type: CredentialAPIKey, Key: "sk-another-long-secret"}
	if otherKey := deriveAccountKey(other); otherKey == nil || *otherKey == *key {
		t.Fatal("different secrets share a key")
	}
}

// jsonRaw builds a JSON value for provider-owned credential metadata.
func jsonRaw(value string) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}
