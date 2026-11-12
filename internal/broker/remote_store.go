package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/MichaelKinsy/PiG/ai"
)

// remoteStore is a live broker-backed account store. No snapshot, access token
// or refresh token is persisted locally, and network failure never falls back
// to the old local SQLite vault. Close owns no transport or database resources.
type remoteStore struct{ client *Client }

var _ ai.ApplicationCredentialStore = (*remoteStore)(nil)
var _ ai.RemoteOAuthCredentialStore = (*remoteStore)(nil)

// snapshot fetches current enabled accounts through the client's short-lived
// shared view; the response never has refreshes.
func (s *remoteStore) snapshot(ctx context.Context) ([]vaultEntry, error) {
	return s.client.snapshot(ctx)
}

// accountInfos fetches only non-secret metadata for callers that do not need
// a credential. The server keeps token-bearing snapshots on the credential
// paths so list and presence checks stay small.
func (s *remoteStore) accountInfos(ctx context.Context) ([]ai.CredentialAccountInfo, error) {
	var accounts []ai.CredentialAccountInfo
	err := s.client.request(ctx, "GET", "/v1/accounts", "", nil, &accounts)
	return accounts, err
}

// sameKey distinguishes a NULL identity from every named account, including "".
func sameKey(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// read resolves the active or exact account from one fresh snapshot.
func (s *remoteStore) read(ctx context.Context, provider string, key *string, explicit bool) (*ai.Credential, *string, error) {
	entries, err := s.snapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range entries {
		if entry.Info.ProviderID == provider && (explicit && sameKey(entry.Info.AccountKey, key) || !explicit && entry.Info.Active) {
			return &entry.Credential, entry.Info.AccountKey, nil
		}
	}
	return nil, nil, nil
}

// Read returns the broker's active access credential, never a local fallback.
func (s *remoteStore) Read(ctx context.Context, provider string) (*ai.Credential, error) {
	credential, _, err := s.read(ctx, provider, nil, false)
	return credential, err
}

// ReadAccount returns exactly key; nil denotes the identity-less account.
func (s *remoteStore) ReadAccount(ctx context.Context, provider string, key *string) (*ai.Credential, error) {
	credential, _, err := s.read(ctx, provider, key, true)
	return credential, err
}

// List lists enabled provider/type pairs without exposing tokens.
func (s *remoteStore) List(ctx context.Context) ([]ai.CredentialInfo, error) {
	accounts, err := s.accountInfos(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ai.CredentialInfo, 0, len(accounts))
	seen := make(map[ai.CredentialInfo]struct{}, len(accounts))
	for _, account := range accounts {
		info := ai.CredentialInfo{ProviderID: account.ProviderID, Type: account.Type}
		if _, ok := seen[info]; !ok {
			result = append(result, info)
			seen[info] = struct{}{}
		}
	}
	return result, nil
}

// ListAccounts lists the broker's live selected-account metadata.
func (s *remoteStore) ListAccounts(ctx context.Context) ([]ai.CredentialAccountInfo, error) {
	return s.accountInfos(ctx)
}

// operation sends one explicit vault mutation and waits for durable persistence.
func (s *remoteStore) operation(ctx context.Context, request vaultRequest, result any) error {
	if err := s.client.request(ctx, "POST", "/v1/credential", "", request, result); err != nil {
		return err
	}
	s.client.invalidateSnapshot()
	return nil
}

// SaveAccount uploads a new login/import; refresh ownership transfers to host.
func (s *remoteStore) SaveAccount(ctx context.Context, provider string, key *string, credential ai.Credential) ([]ai.CredentialAccountInfo, error) {
	var result []ai.CredentialAccountInfo
	err := s.operation(ctx, vaultRequest{Operation: "save", Provider: provider, AccountKey: key, Credential: &credential}, &result)
	return result, err
}

// SelectAccount persists selection on the host for all connected clients.
func (s *remoteStore) SelectAccount(ctx context.Context, provider, key string) (bool, error) {
	var result bool
	err := s.operation(ctx, vaultRequest{Operation: "select", Provider: provider, AccountKey: &key}, &result)
	return result, err
}

// DeleteAccount retires the exact named account on the host.
func (s *remoteStore) DeleteAccount(ctx context.Context, provider, key string) (bool, error) {
	return s.DeleteAccountIdentity(ctx, provider, &key)
}

// DeleteAccountIdentity retires one identity on the host; nil names its NULL row.
func (s *remoteStore) DeleteAccountIdentity(ctx context.Context, provider string, key *string) (bool, error) {
	var result bool
	err := s.operation(ctx, vaultRequest{Operation: "delete-account", Provider: provider, AccountKey: key}, &result)
	return result, err
}

// Delete retires every stored account for provider on the host.
func (s *remoteStore) Delete(ctx context.Context, provider string) error {
	return s.operation(ctx, vaultRequest{Operation: "delete", Provider: provider}, nil)
}

// modify evaluates a non-refresh callback once and submits a CAS write. Refresh
// resolution uses RefreshOAuth instead: a provider callback must never receive
// the sentinel as if it were a usable refresh token. CAS conflicts are reported,
// not retried, so a newly completed login is never blindly replayed.
func (s *remoteStore) modify(ctx context.Context, provider string, key *string, explicit bool, fn func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
	current, _, err := s.read(ctx, provider, key, explicit)
	if err != nil || explicit && current == nil {
		return nil, err
	}
	// Keep an immutable baseline even if the caller mutates its input in place.
	var expected *ai.Credential
	if current != nil {
		data, err := json.Marshal(current)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &expected); err != nil {
			return nil, err
		}
	}
	next, err := fn(current)
	if err != nil {
		return nil, err
	}
	if next == nil {
		return current, nil
	}
	if next.Refresh == remoteRefreshSentinel {
		return nil, fmt.Errorf("remote OAuth credentials must refresh on the broker")
	}
	var result *ai.Credential
	err = s.operation(ctx, vaultRequest{Operation: "modify", Provider: provider, AccountKey: key, Explicit: explicit, Credential: next, Expected: expected}, &result)
	return result, err
}

// Modify saves a login to the selected provider using a version-checked write.
func (s *remoteStore) Modify(ctx context.Context, provider string, fn func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
	return s.modify(ctx, provider, nil, false, fn)
}

// ModifyAccount applies a version-checked non-refresh edit to an exact account.
func (s *remoteStore) ModifyAccount(ctx context.Context, provider string, key *string, fn func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
	return s.modify(ctx, provider, key, true, fn)
}

// RefreshOAuth requests a host-owned refresh of the observed account version.
// A selection change cannot redirect a refresh to a different account.
func (s *remoteStore) RefreshOAuth(ctx context.Context, provider string, key *string, explicit bool, expected ai.Credential) (*ai.Credential, error) {
	current, resolved, err := s.read(ctx, provider, key, explicit)
	if err != nil || current == nil {
		return nil, err
	}
	if !sameRedacted(current, &expected) {
		return current, nil
	}
	var result *ai.Credential
	err = s.client.request(ctx, "POST", "/v1/credential/refresh", "", vaultRequest{Provider: provider, AccountKey: resolved, Explicit: true, Expected: &expected}, &result)
	if err == nil {
		s.client.invalidateSnapshot()
	}
	return result, err
}

// GetAccountBlockedUntil reads shared quota state from the canonical store.
func (s *remoteStore) GetAccountBlockedUntil(ctx context.Context, provider string, key *string, scopes []string) (*int64, error) {
	var result *int64
	err := s.operation(ctx, vaultRequest{Operation: "blocked", Provider: provider, AccountKey: key, Scopes: scopes}, &result)
	return result, err
}

// BlockAccount records a longest-deadline-wins block shared by every client.
func (s *remoteStore) BlockAccount(ctx context.Context, provider string, key *string, scope string, untilMs int64) error {
	return s.operation(ctx, vaultRequest{Operation: "block", Provider: provider, AccountKey: key, Scope: scope, UntilMs: untilMs}, nil)
}

// ClearAccountBlocks clears exact scopes, or every scope when scopes is empty.
func (s *remoteStore) ClearAccountBlocks(ctx context.Context, provider string, key *string, scopes []string) error {
	return s.operation(ctx, vaultRequest{Operation: "clear-blocks", Provider: provider, AccountKey: key, Scopes: scopes}, nil)
}

// AccountCount reports enabled broker accounts, never the stale local count.
func (s *remoteStore) AccountCount() (int, error) {
	var count int
	err := s.client.request(context.Background(), "GET", "/v1/account-count", "", nil, &count)
	return count, err
}

// GetRaw returns the active access credential as exposed by the broker.
func (s *remoteStore) GetRaw(provider string) (ai.Credential, bool, error) {
	credential, err := s.Read(context.Background(), provider)
	if credential == nil {
		return ai.Credential{}, false, err
	}
	return *credential, true, err
}

// HasProvider reports whether any enabled account exists on the host.
func (s *remoteStore) HasProvider(ctx context.Context, provider string) (bool, error) {
	var present bool
	err := s.client.request(ctx, "GET", "/v1/provider?name="+url.QueryEscape(provider), "", nil, &present)
	return present, err
}

// Close releases no resources: the connection is owned by the application.
func (s *remoteStore) Close() error { return nil }
