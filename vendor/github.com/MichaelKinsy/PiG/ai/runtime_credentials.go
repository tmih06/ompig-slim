package ai

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// RuntimeCredentials is a credential store overlay for non-persistent runtime
// API keys (--api-key). An override reads as an api_key credential and is
// never written to the base store. Mirrors upstream
// packages/coding-agent/src/core/runtime-credentials.ts RuntimeCredentials.
type RuntimeCredentials struct {
	store CredentialStore

	mu        sync.RWMutex
	overrides map[string]string
	// order keeps override insertion order, as a JavaScript Map iterates:
	// replacing a key keeps its position, and removing it drops it.
	order []string
}

// NewRuntimeCredentials overlays runtime API keys on store. A nil store holds
// no persistent credentials.
func NewRuntimeCredentials(store CredentialStore) *RuntimeCredentials {
	return &RuntimeCredentials{store: store, overrides: map[string]string{}}
}

// SetRuntimeAPIKey installs a non-persistent API key for providerID.
func (c *RuntimeCredentials) SetRuntimeAPIKey(providerID, apiKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.overrides[providerID]; !exists {
		c.order = append(c.order, providerID)
	}
	c.overrides[providerID] = apiKey
}

// RemoveRuntimeAPIKey drops providerID's runtime key, revealing the base
// store's credential.
func (c *RuntimeCredentials) RemoveRuntimeAPIKey(providerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.overrides[providerID]; exists {
		delete(c.overrides, providerID)
		c.order = slices.DeleteFunc(c.order, func(id string) bool { return id == providerID })
	}
}

// HasRuntimeAPIKey reports whether providerID has a runtime key.
func (c *RuntimeCredentials) HasRuntimeAPIKey(providerID string) bool {
	_, ok := c.RuntimeAPIKey(providerID)
	return ok
}

// RuntimeAPIKey returns providerID's runtime key.
func (c *RuntimeCredentials) RuntimeAPIKey(providerID string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	key, ok := c.overrides[providerID]
	return key, ok
}

// Read implements CredentialStore: a runtime key reads as an api_key credential ahead of the base store. Cancellation returns the caller's cause.
func (c *RuntimeCredentials) Read(ctx context.Context, providerID string) (*Credential, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if key, ok := c.RuntimeAPIKey(providerID); ok && key != "" {
		return &Credential{Type: CredentialAPIKey, Key: key}, nil
	}
	if c.store == nil {
		return nil, nil
	}
	return c.store.Read(ctx, providerID)
}

// List implements CredentialStore: base entries, with every runtime key listed as an api_key entry. It checks cancellation after the base list completes and preserves the caller's cause.
func (c *RuntimeCredentials) List(ctx context.Context) ([]CredentialInfo, error) {
	entries := map[string]CredentialInfo{}
	var order []string
	if c.store != nil {
		base, err := c.store.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, entry := range base {
			if _, seen := entries[entry.ProviderID]; !seen {
				order = append(order, entry.ProviderID)
			}
			entries[entry.ProviderID] = entry
		}
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	c.mu.RLock()
	for _, providerID := range c.order {
		if _, seen := entries[providerID]; !seen {
			order = append(order, providerID)
		}
		entries[providerID] = CredentialInfo{ProviderID: providerID, Type: CredentialAPIKey}
	}
	c.mu.RUnlock()
	infos := make([]CredentialInfo, 0, len(order))
	for _, providerID := range order {
		infos = append(infos, entries[providerID])
	}
	return infos, nil
}

// Modify implements CredentialStore by delegating to the base store; the
// runtime key is unaffected.
func (c *RuntimeCredentials) Modify(ctx context.Context, providerID string, fn func(current *Credential) (*Credential, error)) (*Credential, error) {
	if c.store == nil {
		return nil, errors.New("runtime credentials: no base credential store")
	}
	return c.store.Modify(ctx, providerID, fn)
}

// Delete removes providerID's credential from the base store, then its
// runtime key. Failed or cancelled deletion leaves the runtime key intact and preserves the cancellation cause.
func (c *RuntimeCredentials) Delete(ctx context.Context, providerID string) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if c.store != nil {
		if err := c.store.Delete(ctx, providerID); err != nil {
			return err
		}
	}
	c.RemoveRuntimeAPIKey(providerID)
	return nil
}

// ErrAccountCredentialsUnsupported reports a multi-account operation on a store that holds one
// credential per provider. Mirrors upstream's absent account methods: callers that treat their
// absence as "one account per provider" get this error instead.
var ErrAccountCredentialsUnsupported = errors.New("credential store does not support multiple accounts")

// AccountStore unwraps the runtime-key overlay and reports the base store's multi-account
// capability, so callers can assert an overlay around a single-account store is not multi-account.
func (c *RuntimeCredentials) AccountStore() (AccountCredentialStore, bool) {
	if c == nil {
		return nil, false
	}
	if overlay, ok := c.store.(*RuntimeCredentials); ok {
		return overlay.AccountStore()
	}
	store, ok := c.store.(AccountCredentialStore)
	return store, ok && store != nil
}

func (c *RuntimeCredentials) accountStore() (AccountCredentialStore, error) {
	if store, ok := c.AccountStore(); ok {
		return store, nil
	}
	return nil, ErrAccountCredentialsUnsupported
}

// ReadAccount implements AccountCredentialStore: a runtime key masks the store's account, as it
// masks the provider in Read, and every other read forwards the account key unchanged.
func (c *RuntimeCredentials) ReadAccount(ctx context.Context, providerID string, accountKey *string) (*Credential, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if key, ok := c.RuntimeAPIKey(providerID); ok && key != "" {
		return &Credential{Type: CredentialAPIKey, Key: key}, nil
	}
	store, err := c.accountStore()
	if err != nil {
		return nil, err
	}
	return store.ReadAccount(ctx, providerID, accountKey)
}

// ListAccounts forwards to the base store; the runtime overlay holds provider keys, not accounts.
func (c *RuntimeCredentials) ListAccounts(ctx context.Context) ([]CredentialAccountInfo, error) {
	store, err := c.accountStore()
	if err != nil {
		return nil, err
	}
	return store.ListAccounts(ctx)
}

// SaveAccount forwards to the base store.
func (c *RuntimeCredentials) SaveAccount(ctx context.Context, providerID string, accountKey *string, credential Credential) ([]CredentialAccountInfo, error) {
	store, err := c.accountStore()
	if err != nil {
		return nil, err
	}
	return store.SaveAccount(ctx, providerID, accountKey, credential)
}

// SelectAccount forwards to the base store.
func (c *RuntimeCredentials) SelectAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
	store, err := c.accountStore()
	if err != nil {
		return false, err
	}
	return store.SelectAccount(ctx, providerID, accountKey)
}

// DeleteAccount forwards to the base store.
func (c *RuntimeCredentials) DeleteAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
	store, err := c.accountStore()
	if err != nil {
		return false, err
	}
	return store.DeleteAccount(ctx, providerID, accountKey)
}

// ModifyAccount forwards the account-keyed read-modify-write to the base store, which keeps its
// refresh lease and its exact-row semantics (a nil key is the identity-less row, never the active one).
func (c *RuntimeCredentials) ModifyAccount(ctx context.Context, providerID string, accountKey *string, fn func(current *Credential) (*Credential, error)) (*Credential, error) {
	store, err := c.accountStore()
	if err != nil {
		return nil, err
	}
	return store.ModifyAccount(ctx, providerID, accountKey, fn)
}

// GetAccountBlockedUntil forwards to the base store.
func (c *RuntimeCredentials) GetAccountBlockedUntil(ctx context.Context, providerID string, accountKey *string, scopes []string) (*int64, error) {
	store, err := c.accountStore()
	if err != nil {
		return nil, err
	}
	return store.GetAccountBlockedUntil(ctx, providerID, accountKey, scopes)
}

// BlockAccount forwards to the base store.
func (c *RuntimeCredentials) BlockAccount(ctx context.Context, providerID string, accountKey *string, scope string, untilMs int64) error {
	store, err := c.accountStore()
	if err != nil {
		return err
	}
	return store.BlockAccount(ctx, providerID, accountKey, scope, untilMs)
}

// AccountCount forwards to the base store.
func (c *RuntimeCredentials) AccountCount() (int, error) {
	store, err := c.accountStore()
	if err != nil {
		return 0, err
	}
	return store.AccountCount()
}
