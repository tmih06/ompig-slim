package ai

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/MichaelKinsy/PiG/extensions/sdk/json"
)

// CredentialStoreSync optionally coordinates local credential mutations with a
// hosted store. Set once before opening runtimes. The callback must invoke run
// synchronously at most once, pull before run, and publish before returning.
// Provider requests and OAuth flows still execute on this machine.
var CredentialStoreSync func(context.Context, *SQLiteAuthStorage, func(context.Context) error) error

type credentialSyncContextKey struct{}

// synchronizedCredentialWrite bypasses recursive wrappers within one mutation;
// the host callback owns cross-machine exclusion and persistence failures.
func synchronizedCredentialWrite[T any](ctx context.Context, s *SQLiteAuthStorage, run func(context.Context) (T, error)) (T, error) {
	if CredentialStoreSync == nil || ctx.Value(credentialSyncContextKey{}) != nil {
		return run(ctx)
	}
	var result T
	err := CredentialStoreSync(ctx, s, func(inner context.Context) error {
		var err error
		result, err = run(context.WithValue(inner, credentialSyncContextKey{}, true))
		return err
	})
	return result, err
}

// SaveAccount coordinates a client-side login/import with hosted credentials.
func (s *SQLiteAuthStorage) SaveAccount(ctx context.Context, provider string, key *string, credential Credential) ([]CredentialAccountInfo, error) {
	return synchronizedCredentialWrite(ctx, s, func(ctx context.Context) ([]CredentialAccountInfo, error) {
		return s.saveAccountLocal(ctx, provider, key, credential)
	})
}

// SelectAccount persists a user's account selection across connected clients.
func (s *SQLiteAuthStorage) SelectAccount(ctx context.Context, provider, key string) (bool, error) {
	return synchronizedCredentialWrite(ctx, s, func(ctx context.Context) (bool, error) { return s.selectAccountLocal(ctx, provider, key) })
}

// DeleteAccount publishes a tombstone rather than letting a stale client revive it.
func (s *SQLiteAuthStorage) DeleteAccount(ctx context.Context, provider, key string) (bool, error) {
	return synchronizedCredentialWrite(ctx, s, func(ctx context.Context) (bool, error) { return s.deleteAccountLocal(ctx, provider, key) })
}

// Delete synchronizes provider-wide logout; no provider logout request is proxied.
func (s *SQLiteAuthStorage) Delete(ctx context.Context, provider string) error {
	_, err := synchronizedCredentialWrite(ctx, s, func(ctx context.Context) (bool, error) { return true, s.deleteLocal(ctx, provider) })
	return err
}

// modify pulls the latest rotation under hosted exclusion before the original
// local refresh callback sees it. The callback must never be replayed on conflict.
func (s *SQLiteAuthStorage) modify(ctx context.Context, provider string, key *string, explicit bool, fn func(*Credential) (*Credential, error)) (*Credential, error) {
	return synchronizedCredentialWrite(ctx, s, func(ctx context.Context) (*Credential, error) { return s.modifyLocal(ctx, provider, key, explicit, fn) })
}

// syncRemovalCause is the tombstone cause for an account another machine removed.
const syncRemovalCause = "removed by sync"

// sameCause reports whether a row's tombstone equals the snapshot's, so an
// unchanged row is skipped instead of being rewritten every pull.
func sameCause(row credentialRow, cause *string) bool {
	if !row.DisabledCause.Valid || cause == nil {
		return !row.DisabledCause.Valid && cause == nil
	}
	return row.DisabledCause.String == *cause
}

// sameCredential reports whether the stored row already carries the snapshot
// content, comparing the columns the store itself treats as mutable state.
func sameCredential(row credentialRow, next CredentialSyncRow) bool {
	return row.CredentialType == next.Type && row.Data == next.Data &&
		row.CreatedAt == next.CreatedAt && sameCause(row, next.Disabled)
}

// CredentialSyncRow is a logical account snapshot, not a database copy. Identity
// and tombstones travel; local IDs, quota blocks and refresh leases do not.
type CredentialSyncRow struct {
	Provider  string  `json:"provider"`
	Type      string  `json:"type"`
	Data      string  `json:"data"`
	Identity  *string `json:"identity"`
	Disabled  *string `json:"disabled"`
	CreatedAt int64   `json:"createdAt"`
	Selected  bool    `json:"selected"`
}

// ExportCredentials exports raw stored values without running environment or
// shell substitutions. Its caller must hold the hosted mutation lease.
func (s *SQLiteAuthStorage) ExportCredentials(ctx context.Context) ([]byte, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+rowColumns+` FROM auth_credentials ORDER BY id`)
	if err != nil {
		return nil, err
	}
	type entry struct {
		row  credentialRow
		sync CredentialSyncRow
	}
	var entries []entry
	for rows.Next() {
		row, err := scanCredentialRow(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		r := CredentialSyncRow{Provider: row.Provider, Type: row.CredentialType, Data: row.Data, CreatedAt: row.CreatedAt}
		if row.IdentityKey.Valid {
			r.Identity = &row.IdentityKey.String
		}
		if row.DisabledCause.Valid {
			r.Disabled = &row.DisabledCause.String
		}
		entries = append(entries, entry{row: row, sync: r})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := make([]CredentialSyncRow, 0, len(entries))
	for _, e := range entries {
		selected, ok, err := s.selectedID(ctx, e.row.Provider)
		if err != nil {
			return nil, err
		}
		e.sync.Selected = ok && selected == e.row.ID
		result = append(result, e.sync)
	}
	return json.Marshal(result)
}

// ImportCredentials applies an authoritative snapshot in one local transaction.
// Existing ids survive to preserve local quota references; an account absent from
// the snapshot is tombstoned once (keeping its row for listings) and its secret
// is dropped. Rows whose snapshot content already matches are left untouched, so
// a periodic pull does not churn updated_at or OMP's change observers.
func (s *SQLiteAuthStorage) ImportCredentials(ctx context.Context, data []byte) error {
	var rows []CredentialSyncRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}
	for _, r := range rows {
		if r.Provider == "" || !json.Valid([]byte(r.Data)) {
			return fmt.Errorf("invalid credential snapshot")
		}
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		// The account key is what this store identifies a row by, so the snapshot
		// replaces exactly the rows it names and retires the ones it omits.
		type accountKey struct {
			provider string
			identity string
			present  bool
		}
		keyOf := func(provider string, identity *string) accountKey {
			key := accountKey{provider: provider}
			if identity != nil {
				key.identity, key.present = *identity, true
			}
			return key
		}
		incoming := make(map[accountKey]bool, len(rows))
		for _, r := range rows {
			incoming[keyOf(r.Provider, r.Identity)] = true
		}
		existing := make(map[accountKey]credentialRow, len(rows))
		var stale []credentialRow
		result, err := tx.QueryContext(ctx, `SELECT `+rowColumns+` FROM auth_credentials ORDER BY id`)
		if err != nil {
			return err
		}
		for result.Next() {
			row, scanErr := scanCredentialRow(result)
			if scanErr != nil {
				result.Close()
				return scanErr
			}
			key := keyOf(row.Provider, &row.IdentityKey.String)
			if !row.IdentityKey.Valid {
				key = keyOf(row.Provider, nil)
			}
			if _, duplicate := existing[key]; duplicate || !incoming[key] {
				// A duplicate identity is not addressable by the snapshot: retire it
				// rather than leave two rows competing for one account key.
				stale = append(stale, row)
				continue
			}
			existing[key] = row
		}
		err = result.Err()
		result.Close()
		if err != nil {
			return err
		}
		for _, row := range stale {
			if row.DisabledCause.Valid && row.DisabledCause.String == syncRemovalCause && row.Data == "{}" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE auth_credentials
				SET disabled_cause = ?, data = '{}', updated_at = `+sqliteNow+`
				WHERE id = ?`, syncRemovalCause, row.ID); err != nil {
				return err
			}
		}
		for _, r := range rows {
			key := keyOf(r.Provider, r.Identity)
			if row, ok := existing[key]; ok {
				if sameCredential(row, r) {
					continue
				}
				if _, err := tx.ExecContext(ctx, `UPDATE auth_credentials
					SET credential_type = ?, data = ?, disabled_cause = ?, created_at = ?, updated_at = `+sqliteNow+`
					WHERE id = ?`, r.Type, r.Data, r.Disabled, r.CreatedAt, row.ID); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO auth_credentials
				(provider, credential_type, data, identity_key, disabled_cause, created_at)
				VALUES (?, ?, ?, ?, ?, ?)`, r.Provider, r.Type, r.Data, r.Identity, r.Disabled, r.CreatedAt); err != nil {
				return err
			}
		}
		// Selection is applied last through the store's own key, so a snapshot row
		// inserted above is addressable and an already-correct selection is reused.
		for _, r := range rows {
			if !r.Selected || r.Disabled != nil {
				continue
			}
			id, ok, err := rowIDByAccountKey(ctx, tx, r.Provider, r.Identity)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			// Read the selection through tx: the pool holds no free connection
			// while this transaction is open, so a pool query would deadlock.
			selected, hasSelected, err := selectedIDOn(ctx, tx, r.Provider)
			if err != nil {
				return err
			}
			if hasSelected && selected == id {
				continue
			}
			if err = s.setSelected(ctx, tx, r.Provider, id); err != nil {
				return err
			}
		}
		return nil
	})
}
