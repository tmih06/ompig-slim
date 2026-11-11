package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// remoteRefreshSentinel marks broker ownership, never a usable refresh token.
const remoteRefreshSentinel = "__OMPIG_REMOTE_REFRESH__"

// vaultEntry carries an enabled account and its redacted request credential.
type vaultEntry struct {
	Info       ai.CredentialAccountInfo `json:"info"`
	Credential ai.Credential            `json:"credential"`
}

// vaultRequest addresses accounts by their stable provider/identity, not a
// client-local SQLite id. Explicit distinguishes nil identity from selection.
type vaultRequest struct {
	Operation  string         `json:"operation"`
	Provider   string         `json:"provider"`
	AccountKey *string        `json:"accountKey"`
	Explicit   bool           `json:"explicit"`
	Credential *ai.Credential `json:"credential,omitempty"`
	Expected   *ai.Credential `json:"expected,omitempty"`
	Scope      string         `json:"scope,omitempty"`
	Scopes     []string       `json:"scopes,omitempty"`
	UntilMs    int64          `json:"untilMs,omitempty"`
}

// redact reconstructs OAuth JSON before replacing refresh, so an opaque stored
// representation cannot re-emit its original refresh token during marshaling.
func redact(credential ai.Credential) (ai.Credential, error) {
	data, err := json.Marshal(credential)
	if err != nil {
		return ai.Credential{}, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return ai.Credential{}, err
	}
	if credential.Type == ai.CredentialOAuth {
		fields["refresh"], _ = json.Marshal(remoteRefreshSentinel)
	}
	data, err = json.Marshal(fields)
	if err != nil {
		return ai.Credential{}, err
	}
	var result ai.Credential
	err = json.Unmarshal(data, &result)
	return result, err
}

// entries reads enabled credentials from the canonical vault. API-key config
// references resolve on the broker host; disabled rows and raw refreshes do not
// cross this boundary. The bulk read keeps one snapshot from issuing one SQLite
// query per account, which matters when several TUI requests arrive together.
func (s *Store) entries(ctx context.Context) ([]vaultEntry, error) {
	accounts, err := s.vault.ReadAccounts(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]vaultEntry, 0, len(accounts))
	for _, account := range accounts {
		safe, err := redact(account.Credential)
		if err != nil {
			return nil, err
		}
		result = append(result, vaultEntry{Info: account.Info, Credential: safe})
	}
	return result, nil
}

// snapshot exposes access credentials to trusted direct clients, never refresh
// material. It is live and uncached, so logout and rotations need no disk cache.
func (s *Store) snapshot(w http.ResponseWriter, r *http.Request) {
	entries, err := s.entries(r.Context())
	if err != nil {
		http.Error(w, "vault read failed", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// decodeVaultRequest bounds and validates a single operation without echoing
// credential-bearing bodies or provider errors into HTTP responses or logs.
func decodeVaultRequest(w http.ResponseWriter, r *http.Request) (vaultRequest, bool) {
	var request vaultRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxItemBytes))
	dec.DisallowUnknownFields()
	if dec.Decode(&request) != nil || dec.Decode(new(any)) != io.EOF || request.Provider == "" {
		http.Error(w, "invalid credential request", 400)
		return request, false
	}
	return request, true
}

var errVaultConflict = errors.New("credential changed")

// sameRedacted compares the version visible to a client; a concurrent login,
// refresh or logout must not be overwritten by a callback computed on old data.
func sameRedacted(current, expected *ai.Credential) bool {
	if current == nil || expected == nil {
		return current == nil && expected == nil
	}
	safe, err := redact(*current)
	if err != nil {
		return false
	}
	a, err := json.Marshal(safe)
	if err != nil {
		return false
	}
	b, err := json.Marshal(expected)
	return err == nil && bytes.Equal(a, b)
}

// credential persists account edits and quota blocks on the broker. Generic
// modifications use CAS, never replay a client callback, and cannot write the
// refresh sentinel back into the authoritative vault.
func (s *Store) credential(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeVaultRequest(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var result any
	var err error
	switch request.Operation {
	case "save", "modify", "import":
		if request.Credential == nil || (request.Credential.Type != ai.CredentialOAuth && request.Credential.Type != ai.CredentialAPIKey) || request.Credential.Refresh == remoteRefreshSentinel {
			http.Error(w, "invalid credential", 400)
			return
		}
		if request.Operation == "import" {
			result, err = s.vault.ImportBrokerAccount(ctx, request.Provider, request.AccountKey, *request.Credential)
		} else if request.Operation == "save" {
			result, err = s.vault.SaveAccount(ctx, request.Provider, request.AccountKey, *request.Credential)
		} else {
			modify := func(current *ai.Credential) (*ai.Credential, error) {
				if !sameRedacted(current, request.Expected) {
					return nil, errVaultConflict
				}
				return request.Credential, nil
			}
			var updated *ai.Credential
			if request.Explicit {
				updated, err = s.vault.ModifyAccount(ctx, request.Provider, request.AccountKey, modify)
			} else {
				updated, err = s.vault.Modify(ctx, request.Provider, modify)
			}
			if updated != nil && err == nil {
				result, err = redact(*updated)
			}
		}
	case "select", "delete-account":
		if request.AccountKey == nil {
			http.Error(w, "account key required", 400)
			return
		}
		if request.Operation == "select" {
			result, err = s.vault.SelectAccount(ctx, request.Provider, *request.AccountKey)
		} else {
			result, err = s.vault.DeleteAccount(ctx, request.Provider, *request.AccountKey)
		}
	case "delete":
		err = s.vault.Delete(ctx, request.Provider)
	case "blocked":
		result, err = s.vault.GetAccountBlockedUntil(ctx, request.Provider, request.AccountKey, request.Scopes)
	case "block":
		err = s.vault.BlockAccount(ctx, request.Provider, request.AccountKey, request.Scope, request.UntilMs)
	case "clear-blocks":
		err = s.vault.ClearAccountBlocks(ctx, request.Provider, request.AccountKey, request.Scopes)
	default:
		http.Error(w, "unknown credential operation", 400)
		return
	}
	if errors.Is(err, errVaultConflict) {
		http.Error(w, "credential changed", 412)
		return
	}
	if err != nil {
		http.Error(w, "vault operation failed", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// refresh rechecks the observed version under SQLite's refresh lease. Provider
// exchange and persistence continue even when the waiting HTTP client disconnects.
// A duplicate request reuses the first rotation instead of spending it again.
func (s *Store) refresh(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeVaultRequest(w, r)
	if !ok {
		return
	}
	if !request.Explicit || request.Expected == nil {
		http.Error(w, "explicit account and observed token required", 400)
		return
	}
	credential, err := ai.RefreshVaultCredential(r.Context(), s.vault, request.Provider, request.AccountKey, request.Expected)
	if err != nil {
		http.Error(w, "OAuth refresh failed", 502)
		return
	}
	var result *ai.Credential
	if credential != nil {
		safe, err := redact(*credential)
		if err != nil {
			http.Error(w, "vault read failed", 500)
			return
		}
		result = &safe
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// RunRefresher sweeps immediately and every minute, without overlapping sweeps.
// It skips API keys and uses the same leases/version checks as manual refresh.
// Provider failures retain credentials for retry except the provider-specific
// definitive-failure retirement already implemented by the SQLite store.
func (s *Store) RunRefresher(ctx context.Context, report func(error)) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.sweep(ctx); err != nil && ctx.Err() == nil {
			report(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// sweep refreshes near-expiry accounts sequentially; a failed account must not
// prevent other providers from rotating. No credential details enter errors.
func (s *Store) sweep(ctx context.Context) error {
	accounts, err := s.vault.ListAccounts(ctx)
	if err != nil {
		return fmt.Errorf("vault refresher: list failed")
	}
	var failed bool
	for _, account := range accounts {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if account.Type != ai.CredentialOAuth {
			continue
		}
		credential, err := s.vault.ReadAccount(ctx, account.ProviderID, account.AccountKey)
		if err != nil {
			failed = true
			continue
		}
		if credential == nil || credential.ExpiresMillis() > float64(time.Now().Add(5*time.Minute).UnixMilli()) {
			continue
		}
		// RefreshVaultCredential owns the detached timeout and waits through
		// persistence, so shutdown cannot close SQLite during a rotation.
		_, err = ai.RefreshVaultCredential(ctx, s.vault, account.ProviderID, account.AccountKey, nil)
		if err != nil {
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("vault refresher: one or more accounts could not refresh")
	}
	return nil
}
