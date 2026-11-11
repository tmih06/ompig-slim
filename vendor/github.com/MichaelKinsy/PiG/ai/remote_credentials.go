package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ApplicationCredentialStore is the multi-account store owned by an application
// runtime. Local profiles use SQLite; broker profiles must not open a local vault.
type ApplicationCredentialStore interface {
	AccountCredentialStore
	AccountBlockClearer
	Close() error
	GetRaw(string) (Credential, bool, error)
	HasProvider(context.Context, string) (bool, error)
}

// ImportBrokerAccount inserts a locally migrated account only if the identity
// has never existed on this broker. Tombstones count as existing, so retrying
// migration cannot revive logout or overwrite a newer server-side rotation.
func (s *SQLiteAuthStorage) ImportBrokerAccount(ctx context.Context, provider string, key *string, credential Credential) (bool, error) {
	data, err := json.Marshal(credential)
	if err != nil {
		return false, err
	}
	inserted := false
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		_, exists, err := rowIDByAccountKey(ctx, tx, provider, key)
		if err != nil || exists {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO auth_credentials (provider, credential_type, data, identity_key) VALUES (?, ?, ?, ?)", provider, string(credential.Type), string(data), key)
		inserted = err == nil
		return err
	})
	return inserted, err
}

// CredentialStoreFactory overrides discovery for configured broker profiles.
// Install once before opening runtimes; never change it while requests run.
var CredentialStoreFactory func(string) (ApplicationCredentialStore, error)

// RemoteOAuthCredentialStore refreshes at the credential owner, without invoking
// the client's provider callback. expected coalesces concurrent auth-recovery
// requests that observed the same access token; refresh tokens are never sent.
type RemoteOAuthCredentialStore interface {
	RefreshOAuth(context.Context, string, *string, bool, Credential) (*Credential, error)
}

// remoteOAuthStore unwraps runtime API-key overlays without making a local store
// claim remote refresh support. Runtime keys never reach the stored OAuth path.
func remoteOAuthStore(store CredentialStore) (RemoteOAuthCredentialStore, bool) {
	if overlay, ok := store.(*RuntimeCredentials); ok {
		return remoteOAuthStore(overlay.store)
	}
	remote, ok := store.(RemoteOAuthCredentialStore)
	return remote, ok
}

// RefreshVaultCredential uses registered provider logic under the vault's normal
// SQLite lease and CAS. A nil expected token means a proactive five-minute sweep;
// otherwise only the version the caller observed is refreshed. Once exchange
// starts, cancellation cannot discard a rotated token before persistence.
func RefreshVaultCredential(ctx context.Context, store AccountCredentialStore, provider string, key *string, expected *Credential) (*Credential, error) {
	p, ok := GetOAuthProvider(provider)
	if !ok {
		return nil, fmt.Errorf("unknown OAuth provider: %s", provider)
	}
	needsRefresh := func(current Credential) bool {
		if expected != nil {
			return current.Access == expected.Access && current.ExpiresMillis() == expected.ExpiresMillis()
		}
		return current.ExpiresMillis() <= float64(time.Now().Add(5*time.Minute).UnixMilli())
	}
	work, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	return refreshStoredOAuthLocked(work, store, provider, &OAuthAuth{Refresh: oauthRefresh(p)}, needsRefresh, AuthResolutionOverrides{AccountKey: key, AccountKeySet: true})
}
