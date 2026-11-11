package ai

// Multi-account credential storage backed by SQLite.
//
// Ports packages/coding-agent/src/core/sqlite-auth-storage.ts.
//
// The database is shared with OMP (`~/.pi/agent/agent.db`), so this store speaks OMP's schema rather
// than defining its own. That matters: OMP's `auth_credentials` already holds one row per account,
// keyed by `identity_key`, with `disabled_cause` as the tombstone - everything multi-account needs
// except which account is *selected*. A competing schema in the same file cannot work, because
// `CREATE TABLE IF NOT EXISTS` is a no-op against an existing table and any column this store invented
// (`active`) is absent from OMP's, which fails the moment an index mentions it.
//
// OMP's schema is used exactly as OMP defines it, with no added tables or columns:
//   - Enabled rows are `disabled_cause IS NULL`; accounts are distinguished by `identity_key`.
//   - Removal is a `disabled_cause` tombstone, never a `DELETE`, so OMP agrees the row is gone.
//   - OMP has no "active account" column. Its durable default is the provider's oldest enabled row
//     (`ORDER BY id ASC`), with per-session pins layered on top. This store records its own selection
//     in OMP's `cache` table under a namespaced key (see setSelected) and falls back to OMP's default
//     when that record is gone. Rows are never renumbered or reordered: OMP keys blocks and refresh
//     leases by `credential_id`, and their ids are global across providers.
//   - Fallback account policy lives in OMP's own `auth_credential_blocks` (provider key
//     `<provider>:<credential_type>`, one row per account and scope), and OAuth refreshes take OMP's
//     `auth_credential_refresh_leases` so a single-use refresh token is spent by one process only.
//
// Writes run in SQLite transactions, which serialize across processes: no lock file is needed.
// Modify never holds a transaction across its caller's callback: same-provider callbacks queue in
// process, and an OAuth callback holds OMP's refresh lease across processes while it runs.

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/google/uuid"
	_ "modernc.org/sqlite" // The SQLite driver both this store and OMP's database speak.

	"github.com/MichaelKinsy/PiG/extensions/sdk/json"
	"github.com/MichaelKinsy/PiG/internal/configvalue"
	"github.com/MichaelKinsy/PiG/internal/jsstring"
)

// CredentialAccountInfo is non-secret metadata for one stored account. Mirrors upstream
// CredentialAccountInfo (packages/ai/src/auth/types.ts).
type CredentialAccountInfo struct {
	ProviderID string         `json:"providerId"`
	Type       CredentialType `json:"type"`
	// AccountKey is nil for an identity-less credential, which OMP stores with a NULL identity_key.
	AccountKey *string `json:"accountKey"`
	// Label is a human-readable account label (email, account id, or key hint) when the credential
	// exposes one.
	Label string `json:"label,omitempty"`
	// Active marks the account Read(providerID) resolves to.
	Active bool `json:"active"`
	// CreatedAt is the epoch-milliseconds time the credential was stored, or 0 when unknown.
	CreatedAt int64 `json:"createdAt,omitempty"`
}

// AccountCredentialStore is credential storage that holds several accounts per provider. Read
// keeps returning the active credential, so every single-account caller is unaffected; these
// methods expose the rest. Stores without multi-account support simply do not implement it.
//
// A nil accountKey names the identity-less row (OMP's `identity_key IS NULL`), never the active
// one: only Read/Modify without a key resolve the provider's selection.
type AccountCredentialStore interface {
	CredentialStore

	// ReadAccount returns the credential of exactly accountKey, resolved like Read.
	ReadAccount(ctx context.Context, providerID string, accountKey *string) (*Credential, error)
	// ListAccounts lists every enabled account, active first per provider.
	ListAccounts(ctx context.Context) ([]CredentialAccountInfo, error)
	// SaveAccount stores credential as accountKey (reviving a tombstone) and makes it active.
	SaveAccount(ctx context.Context, providerID string, accountKey *string, credential Credential) ([]CredentialAccountInfo, error)
	// SelectAccount makes accountKey the account Read resolves to.
	SelectAccount(ctx context.Context, providerID, accountKey string) (bool, error)
	// DeleteAccount tombstones one account, keeping the provider's others.
	DeleteAccount(ctx context.Context, providerID, accountKey string) (bool, error)
	// ModifyAccount is Modify for one named account, under the same refresh lease.
	ModifyAccount(ctx context.Context, providerID string, accountKey *string, fn func(current *Credential) (*Credential, error)) (*Credential, error)
	// GetAccountBlockedUntil reports the latest live block deadline among scopes, including the
	// unscoped (\"\") block, or nil when the account has none.
	GetAccountBlockedUntil(ctx context.Context, providerID string, accountKey *string, scopes []string) (*int64, error)
	// BlockAccount records a block that never shortens an existing deadline.
	BlockAccount(ctx context.Context, providerID string, accountKey *string, scope string, untilMs int64) error
	// AccountCount reports the number of enabled credentials across all providers.
	AccountCount() (int, error)
}

// AccountBlockClearer is the optional capability of dropping an account's recorded blocks. A
// spent saved reset makes the windows it covers whole again, so the block recorded for the old
// window must go or the account stays retired until that window ends.
//
// It is deliberately not part of AccountCredentialStore: a store that cannot clear a block keeps
// its blocks, which is the safe direction (the account is simply probed less eagerly).
type AccountBlockClearer interface {
	// ClearAccountBlocks drops every scope's block for one account, or exactly the named scopes
	// when scopes is non-empty. An empty scope string is the unscoped row, which applies to every
	// request.
	ClearAccountBlocks(ctx context.Context, providerID string, accountKey *string, scopes []string) error
}

// ErrCredentialStoreReadOnly reports a write to a store opened for inspection.
var ErrCredentialStoreReadOnly = errors.New("Credential store is readonly (opened for inspection)")

const (
	// logoutCause is OMP's tombstone value for a credential the user logged out of.
	logoutCause = "logout"
	// replacedByOAuthCause is OMP's tombstone for an API key replaced by an OAuth login.
	replacedByOAuthCause = "replaced by oauth login"
)

// OMP's OAuth refresh lease: how long a holder may stay silent, how often this store renews it,
// and how long a peer waits for it before giving up rather than spending the single-use token.
const (
	refreshLeaseTTLMs      = 15_000
	refreshLeaseRenewMs    = 5_000
	refreshLeasePollMs     = 50
	refreshLeasePollMaxMs  = 250
	refreshLeaseWaitMaxMs  = 10_000
	sqliteBusyTimeoutMs    = 5_000
	selectionCacheLifetime = 10 * 365 * 24 * 60 * 60
)

// sqliteNow is OMP's epoch-seconds column default. Blocks and leases use milliseconds.
const sqliteNow = "CAST(strftime('%s','now') AS INTEGER)"

// rowColumns is the column list of every credential row read, so a new column cannot be missed in
// one query.
const rowColumns = "id, provider, credential_type, data, identity_key, disabled_cause, created_at"

// ompAuthDDL is what OMP itself creates; listed only so a fresh database matches what OMP expects.
var ompAuthDDL = []string{
	`CREATE TABLE IF NOT EXISTS auth_credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider TEXT NOT NULL,
		credential_type TEXT NOT NULL,
		data TEXT NOT NULL,
		disabled_cause TEXT DEFAULT NULL,
		identity_key TEXT DEFAULT NULL,
		created_at INTEGER NOT NULL DEFAULT (` + sqliteNow + `),
		updated_at INTEGER NOT NULL DEFAULT (` + sqliteNow + `)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_auth_provider ON auth_credentials(provider)`,
	// OMP's generic key/value store. Only used here to record which account is selected; OMP prunes
	// it by expiry, so this store writes a far-future expiry to keep its own keys alive.
	`CREATE TABLE IF NOT EXISTS cache (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		expires_at INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_cache_expires ON cache(expires_at)`,
	// OMP's per-account backoff rows. The provider key is the provider and credential type, so a
	// block names one account through its credential id. Written with longest-deadline-wins.
	`CREATE TABLE IF NOT EXISTS auth_credential_blocks (
		credential_id INTEGER NOT NULL,
		provider_key TEXT NOT NULL,
		block_scope TEXT NOT NULL DEFAULT '',
		blocked_until_ms INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		PRIMARY KEY (credential_id, provider_key, block_scope)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_auth_credential_blocks_expires ON auth_credential_blocks(blocked_until_ms)`,
	// OMP's cross-process OAuth refresh lease. One row per credential while a refresh runs, so a
	// single-use refresh token is exchanged once across every process sharing this database.
	`CREATE TABLE IF NOT EXISTS auth_credential_refresh_leases (
		credential_id INTEGER PRIMARY KEY,
		owner TEXT NOT NULL,
		expires_at_ms INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_auth_credential_refresh_leases_expires
		ON auth_credential_refresh_leases(expires_at_ms)`,
}

// credentialRow is one auth_credentials row.
type credentialRow struct {
	ID             int64
	Provider       string
	CredentialType string
	Data           string
	IdentityKey    sql.NullString
	DisabledCause  sql.NullString
	CreatedAt      int64
}

// refreshLease is one held refresh lease row.
type refreshLease struct {
	id    int64
	owner string
}

// querier is the subset of *sql.DB and *sql.Tx these queries use.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SQLiteAuthStorage is credential storage with several accounts per provider, sharing OMP's
// database. It implements CredentialStore and AccountCredentialStore.
type SQLiteAuthStorage struct {
	db       *sql.DB
	path     string
	readOnly bool
	closed   bool

	queuesMu sync.Mutex
	queues   map[string]chan struct{}
}

// OpenSQLiteAuthStorage opens the credential database at path, creating its directory (0700),
// OMP's schema, and OMP's later columns on an older file. The file is owner-only.
func OpenSQLiteAuthStorage(path string) (*SQLiteAuthStorage, error) {
	if path == "" {
		return nil, errors.New("auth: empty credential database path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	store, err := openSQLiteStore(path, false)
	if err != nil {
		return nil, err
	}
	for _, statement := range ompAuthDDL {
		if _, err := store.db.ExecContext(context.Background(), statement); err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("auth: create schema: %w", err)
		}
	}
	if err := store.upgradeAuthCredentials(); err != nil {
		_ = store.Close()
		return nil, err
	}
	// The file holds tokens; keep it owner-only even if it predates this store.
	chmodOwnerOnly(path)
	chmodOwnerOnly(path + "-wal")
	chmodOwnerOnly(path + "-shm")
	return store, nil
}

// OpenReadOnlySQLiteAuthStorage opens an existing credential database without writing to it: no
// directory creation, schema, migrations, chmod, or WAL switch, and SQLite rejects every write.
// Reads still work, including block lookups, so an auth check can report state without touching
// the file. A missing path fails instead of silently reporting an empty store.
func OpenReadOnlySQLiteAuthStorage(path string) (*SQLiteAuthStorage, error) {
	if path == "" {
		return nil, errors.New("auth: empty credential database path")
	}
	return openSQLiteStore(path, true)
}

func openSQLiteStore(path string, readOnly bool) (*SQLiteAuthStorage, error) {
	db, err := sql.Open("sqlite", sqliteDSN(path, readOnly))
	if err != nil {
		return nil, err
	}
	// Node's DatabaseSync is one connection; a single pooled connection keeps that serialization and
	// lets a transaction and the statements inside it see each other.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	// Open eagerly: a missing database must fail here (a read-only open cannot create it) instead
	// of every later read reporting an empty store.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLiteAuthStorage{db: db, path: path, readOnly: readOnly}, nil
}

// sqliteDSN builds the driver URI. Read-only opens ask SQLite itself to refuse writes and skip the
// WAL switch; writable opens set OMP's pragmas on every connection.
func sqliteDSN(path string, readOnly bool) string {
	location := &url.URL{Scheme: "file", Path: path}
	query := url.Values{}
	query.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", sqliteBusyTimeoutMs))
	query.Add("_txlock", "immediate")
	if readOnly {
		query.Add("mode", "ro")
		query.Add("_pragma", "query_only(1)")
	} else {
		query.Add("_pragma", "journal_mode(WAL)")
		query.Add("_pragma", "foreign_keys(1)")
	}
	location.RawQuery = query.Encode()
	return location.String()
}

func chmodOwnerOnly(path string) {
	// Windows and read-only mounts reject POSIX modes; the directory mode still applies.
	_ = os.Chmod(path, 0o600)
}

// upgradeAuthCredentials adds the columns OMP adds in its own migrations, so an older database (or
// one OMP wrote before this version) works here too. Adding a column that exists is skipped.
func (s *SQLiteAuthStorage) upgradeAuthCredentials() error {
	rows, err := s.db.QueryContext(context.Background(), "PRAGMA table_info(auth_credentials)")
	if err != nil {
		return fmt.Errorf("auth: read schema: %w", err)
	}
	columns := map[string]bool{}
	for rows.Next() {
		var (
			cid       int64
			name      string
			kind      string
			notNull   int64
			defaultV  sql.NullString
			primaryID int64
		)
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultV, &primaryID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("auth: read schema: %w", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("auth: read schema: %w", err)
	}
	_ = rows.Close()
	if !columns["identity_key"] {
		if _, err := s.db.ExecContext(context.Background(), "ALTER TABLE auth_credentials ADD COLUMN identity_key TEXT DEFAULT NULL"); err != nil {
			return fmt.Errorf("auth: add identity_key: %w", err)
		}
	}
	if !columns["disabled_cause"] {
		if _, err := s.db.ExecContext(context.Background(), "ALTER TABLE auth_credentials ADD COLUMN disabled_cause TEXT DEFAULT NULL"); err != nil {
			return fmt.Errorf("auth: add disabled_cause: %w", err)
		}
	}
	// The unique index needs identity_key, so create it after the column is guaranteed.
	if _, err := s.db.ExecContext(context.Background(), `CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_provider_identity
		ON auth_credentials(provider, identity_key) WHERE identity_key IS NOT NULL`); err != nil {
		return fmt.Errorf("auth: create identity index: %w", err)
	}
	return nil
}

// Path returns the credential database path.
func (s *SQLiteAuthStorage) Path() string { return s.path }

// ReadOnly reports whether this store was opened for inspection.
func (s *SQLiteAuthStorage) ReadOnly() bool { return s.readOnly }

// Close closes the database. It is idempotent; later calls on the store report sql.ErrConnDone
// rather than panicking.
func (s *SQLiteAuthStorage) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	return s.db.Close()
}

// scanCredentialRow decodes rowColumns and, when supplied, the first selection
// column. A failed scan returns no partial credential row.
func scanCredentialRow(scanner interface{ Scan(dest ...any) error }, selection ...*sql.NullString) (credentialRow, error) {
	var row credentialRow
	dest := [8]any{&row.ID, &row.Provider, &row.CredentialType, &row.Data, &row.IdentityKey, &row.DisabledCause, &row.CreatedAt}
	n := 7
	if len(selection) > 0 {
		dest[n] = selection[0]
		n++
	}
	if err := scanner.Scan(dest[:n]...); err != nil {
		return credentialRow{}, err
	}
	return row, nil
}

// rowsFor returns one provider's enabled rows, oldest first.
func (s *SQLiteAuthStorage) rowsFor(ctx context.Context, providerID string) ([]credentialRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+rowColumns+`
		FROM auth_credentials WHERE provider = ? AND disabled_cause IS NULL ORDER BY id ASC`, providerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []credentialRow
	for rows.Next() {
		row, err := scanCredentialRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// selectionKey is the cache key naming the account this store selected for a provider.
func selectionKey(providerID string) string { return "omp-slim:auth-active:" + providerID }

// setSelected records which account a provider should use.
//
// OMP has no column for this, and its credential ids cannot be renumbered to express it: OMP keys
// auth_credential_blocks and refresh leases by credential_id, so moving a row would break those
// references, and ids are globally unique across providers so one cannot be slid below another.
// The selection therefore lives in OMP's generic cache table under this store's own key. OMP only
// prunes that table by expiry, so a namespaced key is inert to it; if the row is ever pruned, Read
// falls back to OMP's default (the oldest enabled account).
func (s *SQLiteAuthStorage) setSelected(ctx context.Context, q querier, providerID string, credentialID int64) error {
	_, err := q.ExecContext(ctx, `INSERT INTO cache (key, value, expires_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at`,
		selectionKey(providerID), strconv.FormatInt(credentialID, 10), time.Now().Unix()+selectionCacheLifetime)
	return err
}

// selectedID returns the credential id last selected for a provider, when that row is still enabled.
func (s *SQLiteAuthStorage) selectedID(ctx context.Context, providerID string) (int64, bool, error) {
	// A read-only open of a database this store never wrote has no selection record; OMP's oldest
	// enabled row is then the answer, so treat the missing table as no selection.
	present, err := s.hasTable(ctx, "cache")
	if err != nil || !present {
		return 0, false, err
	}
	return selectedIDOn(ctx, s.db, providerID)
}

// selectedIDOn reads the selection through q, so a caller holding a transaction
// never waits on the pool for the connection its own transaction occupies.
func selectedIDOn(ctx context.Context, q querier, providerID string) (int64, bool, error) {
	var value string
	err := q.QueryRowContext(ctx, "SELECT value FROM cache WHERE key = ?", selectionKey(providerID)).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	id, convErr := strconv.ParseInt(jsstring.Trim(value), 10, 64)
	if convErr != nil {
		return 0, false, nil
	}
	return enabledRow(ctx, q, id)
}

// enabledRow reports the credential id when the row is still enabled, so a
// selection naming a tombstoned account reads as no selection.
func enabledRow(ctx context.Context, q querier, id int64) (int64, bool, error) {
	var enabled int64
	err := q.QueryRowContext(ctx, "SELECT id FROM auth_credentials WHERE id = ? AND disabled_cause IS NULL", id).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return enabled, true, nil
}

// exactRow returns the enabled row holding exactly accountKey (including nil, matching OMP's
// identity-less rows). Explicit-account operations never fall back to another identity.
func (s *SQLiteAuthStorage) exactRow(ctx context.Context, providerID string, accountKey *string) (*credentialRow, error) {
	var key any
	if accountKey != nil {
		key = *accountKey
	}
	row, err := scanCredentialRow(s.db.QueryRowContext(ctx, `SELECT `+rowColumns+`
		FROM auth_credentials
		WHERE provider = ? AND identity_key IS ? AND disabled_cause IS NULL
		ORDER BY id ASC LIMIT 1`, providerID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// rowIDByAccountKey returns the row holding exactly accountKey, enabled or tombstoned, oldest
// first. Login-style writes revive that row; refreshes use exactRow, which never touches a tombstone.
func rowIDByAccountKey(ctx context.Context, q querier, providerID string, accountKey *string) (int64, bool, error) {
	var key any
	if accountKey != nil {
		key = *accountKey
	}
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM auth_credentials
		WHERE provider = ? AND identity_key IS ? ORDER BY id ASC LIMIT 1`, providerID, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// hasTable reports whether the shared database has one of OMP's tables; a read-only open cannot
// create them.
func (s *SQLiteAuthStorage) hasTable(ctx context.Context, name string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteAuthStorage) assertWritable() error {
	if s.readOnly {
		return ErrCredentialStoreReadOnly
	}
	return nil
}

// withTx runs fn in an immediate transaction, rolling back on error.
func (s *SQLiteAuthStorage) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// rowFor returns the row an operation applies to: the exact account when one is named, otherwise
// the account Read(providerID) resolves to (recorded selection, else OMP's oldest enabled row).
func (s *SQLiteAuthStorage) rowFor(ctx context.Context, providerID string, accountKey *string) (*credentialRow, error) {
	if accountKey != nil {
		return s.exactRow(ctx, providerID, accountKey)
	}
	// Read only the oldest row and its preference, not every account's token
	// payload. A valid preference needs at most one additional primary-key read.
	hasSelection := true
	if s.readOnly {
		var err error
		hasSelection, err = s.hasTable(ctx, "cache")
		if err != nil {
			return nil, err
		}
	}
	query := `SELECT ` + rowColumns
	args := []any{providerID}
	var preferred sql.NullString
	if hasSelection {
		query += `, (SELECT value FROM cache WHERE key = ?)`
		args = []any{selectionKey(providerID), providerID}
	}
	query += ` FROM auth_credentials WHERE provider = ? AND disabled_cause IS NULL ORDER BY id ASC LIMIT 1`
	scanner := s.db.QueryRowContext(ctx, query, args...)
	var row credentialRow
	var err error
	if hasSelection {
		row, err = scanCredentialRow(scanner, &preferred)
	} else {
		row, err = scanCredentialRow(scanner)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if preferred.Valid {
		if selected, parseErr := strconv.ParseInt(jsstring.Trim(preferred.String), 10, 64); parseErr == nil && selected != row.ID {
			chosen, err := s.rowByID(ctx, selected)
			if err != nil {
				return nil, err
			}
			if chosen != nil && chosen.Provider == providerID && !chosen.DisabledCause.Valid {
				return chosen, nil
			}
		}
	}
	return &row, nil
}

func (s *SQLiteAuthStorage) rowByID(ctx context.Context, id int64) (*credentialRow, error) {
	row, err := scanCredentialRow(s.db.QueryRowContext(ctx, `SELECT `+rowColumns+`
		FROM auth_credentials WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// resolveRowCredential returns a stored row as the caller sees it, with an API key's configured
// value resolved.
func resolveRowCredential(row *credentialRow) *Credential {
	credential := rowCredential(row)
	if credential == nil {
		return nil
	}
	if credential.Type == CredentialAPIKey && credential.Key != "" {
		credential.Key = configvalue.Resolve(credential.Key, credential.Env)
	}
	return credential
}

func rowCredential(row *credentialRow) *Credential {
	if row == nil {
		return nil
	}
	return decodeCredentialData(row.CredentialType, row.Data)
}

// Read implements CredentialStore: the active account's credential.
func (s *SQLiteAuthStorage) Read(ctx context.Context, providerID string) (*Credential, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	row, err := s.rowFor(ctx, providerID, nil)
	if err != nil {
		return nil, err
	}
	return resolveRowCredential(row), context.Cause(ctx)
}

// ReadAccount returns the credential of exactly accountKey (nil: the identity-less row).
func (s *SQLiteAuthStorage) ReadAccount(ctx context.Context, providerID string, accountKey *string) (*Credential, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	row, err := s.exactRow(ctx, providerID, accountKey)
	if err != nil {
		return nil, err
	}
	return resolveRowCredential(row), context.Cause(ctx)
}

// GetRaw returns the active account's credential without resolving a configured API-key value. It
// is the raw read the OAuth refresh helpers use; Read resolves for request auth.
func (s *SQLiteAuthStorage) GetRaw(providerID string) (Credential, bool, error) {
	row, err := s.rowFor(context.Background(), providerID, nil)
	if err != nil || row == nil {
		return Credential{}, false, err
	}
	credential := rowCredential(row)
	if credential == nil {
		return Credential{}, false, nil
	}
	return cloneCredential(*credential), true, nil
}

// List implements CredentialStore without resolving configured key values.
func (s *SQLiteAuthStorage) List(ctx context.Context) ([]CredentialInfo, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT provider, credential_type
		FROM auth_credentials WHERE disabled_cause IS NULL ORDER BY provider ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []CredentialInfo
	for rows.Next() {
		var provider, kind string
		if err := rows.Scan(&provider, &kind); err != nil {
			return nil, err
		}
		out = append(out, CredentialInfo{ProviderID: provider, Type: infoCredentialType(kind)})
	}
	return out, rows.Err()
}

// ListAccounts lists every enabled account, providers and ids ascending, marking the account Read
// resolves to.
func (s *SQLiteAuthStorage) ListAccounts(ctx context.Context) ([]CredentialAccountInfo, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	return s.listAccounts(ctx)
}

func (s *SQLiteAuthStorage) listAccounts(ctx context.Context) ([]CredentialAccountInfo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+rowColumns+`
		FROM auth_credentials WHERE disabled_cause IS NULL ORDER BY provider ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	all := []credentialRow{}
	for rows.Next() {
		row, err := scanCredentialRow(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		all = append(all, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	// Mirrors Read: the recorded selection, else the provider's oldest enabled row, which is the
	// first row of that provider in this id-ordered list.
	activeIDs := map[string]int64{}
	for _, row := range all {
		if _, seen := activeIDs[row.Provider]; seen {
			continue
		}
		selected, ok, err := s.selectedID(ctx, row.Provider)
		if err != nil {
			return nil, err
		}
		if ok {
			activeIDs[row.Provider] = selected
		} else {
			activeIDs[row.Provider] = row.ID
		}
	}
	out := make([]CredentialAccountInfo, 0, len(all))
	for i := range all {
		out = append(out, accountInfo(&all[i], activeIDs[all[i].Provider] == all[i].ID))
	}
	return out, nil
}

// AccountCount reports the number of enabled credentials across all providers.
func (s *SQLiteAuthStorage) AccountCount() (int, error) {
	var count int
	if err := s.db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM auth_credentials WHERE disabled_cause IS NULL").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// HasProvider reports whether any enabled credential is stored for providerID.
func (s *SQLiteAuthStorage) HasProvider(ctx context.Context, providerID string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM auth_credentials
		WHERE provider = ? AND disabled_cause IS NULL LIMIT 1`, providerID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// SaveAccount stores credential as accountKey, reviving a tombstone, and makes it active.
func (s *SQLiteAuthStorage) SaveAccount(ctx context.Context, providerID string, accountKey *string, credential Credential) ([]CredentialAccountInfo, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if err := s.assertWritable(); err != nil {
		return nil, err
	}
	if err := s.saveAccount(ctx, providerID, accountKey, credential); err != nil {
		return nil, err
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	accounts, err := s.listAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CredentialAccountInfo, 0, len(accounts))
	for _, account := range accounts {
		if account.ProviderID == providerID {
			out = append(out, account)
		}
	}
	return out, nil
}

func (s *SQLiteAuthStorage) saveAccount(ctx context.Context, providerID string, accountKey *string, credential Credential) error {
	data, err := encodeCredentialData(credential)
	if err != nil {
		return err
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		// Match on the account identity, and revive a row OMP or this store tombstoned earlier.
		existing, ok, err := rowIDByAccountKey(ctx, tx, providerID, accountKey)
		if err != nil {
			return err
		}
		var rowID int64
		if ok {
			if _, err := tx.ExecContext(ctx, `UPDATE auth_credentials
				SET credential_type = ?, data = ?, disabled_cause = NULL,
				    updated_at = `+sqliteNow+`
				WHERE id = ?`, string(credential.Type), data, existing); err != nil {
				return err
			}
			rowID = existing
		} else {
			var identity any
			if accountKey != nil {
				identity = *accountKey
			}
			if err := tx.QueryRowContext(ctx, `INSERT INTO auth_credentials (provider, credential_type, data, identity_key)
				VALUES (?, ?, ?, ?) RETURNING id`,
				providerID, string(credential.Type), data, identity).Scan(&rowID); err != nil {
				return err
			}
		}
		// Saving an account makes it the provider's default, matching "log in and use it".
		return s.setSelected(ctx, tx, providerID, rowID)
	})
}

// SelectAccount makes accountKey the account Read resolves to.
func (s *SQLiteAuthStorage) SelectAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
	if err := context.Cause(ctx); err != nil {
		return false, err
	}
	if err := s.assertWritable(); err != nil {
		return false, err
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM auth_credentials
		WHERE provider = ? AND identity_key = ? AND disabled_cause IS NULL`, providerID, accountKey).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, context.Cause(ctx)
	}
	if err != nil {
		return false, err
	}
	if err := context.Cause(ctx); err != nil {
		return false, err
	}
	if err := s.withTx(ctx, func(tx *sql.Tx) error { return s.setSelected(ctx, tx, providerID, id) }); err != nil {
		return false, err
	}
	return true, nil
}

// DeleteAccount scrubs and tombstones one account, keeping the provider's others.
// The identity tombstone prevents a later login from silently reviving logout, while
// clearing data removes the no-longer-usable access and refresh credentials.
func (s *SQLiteAuthStorage) DeleteAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
	if err := context.Cause(ctx); err != nil {
		return false, err
	}
	if err := s.assertWritable(); err != nil {
		return false, err
	}
	selected, hasSelected, err := s.selectedID(ctx, providerID)
	if err != nil {
		return false, err
	}
	removed := false
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		// Scrub and tombstone rather than DELETE, so OMP and this store agree the account is gone
		// while the identity stays available to OMP's disabled-credential listing.
		row, err := scanCredentialRow(tx.QueryRowContext(ctx, `SELECT `+rowColumns+`
			FROM auth_credentials
			WHERE provider = ? AND identity_key IS ? AND disabled_cause IS NULL
			ORDER BY id ASC LIMIT 1`, providerID, accountKey))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE auth_credentials
			SET data = '{}', disabled_cause = ?, updated_at = `+sqliteNow+`
			WHERE id = ? AND disabled_cause IS NULL`, logoutCause, row.ID)
		if err != nil {
			return err
		}
		changes, err := result.RowsAffected()
		if err != nil || changes == 0 {
			return err
		}
		removed = true
		// Only the removal of the selected account changes which account is active; deleting another
		// account must leave an explicit selection alone.
		if hasSelected && selected == row.ID {
			return s.clearSelection(ctx, tx, providerID)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return removed, context.Cause(ctx)
}

// Modify implements CredentialStore: serialized read-modify-write for one account.
func (s *SQLiteAuthStorage) Modify(ctx context.Context, providerID string, fn func(current *Credential) (*Credential, error)) (*Credential, error) {
	return s.modifyLocal(ctx, providerID, nil, false, fn)
}

// ModifyAccount is Modify for one named account (nil: the identity-less row). It never touches
// another identity and leaves the provider's selection alone.
func (s *SQLiteAuthStorage) ModifyAccount(ctx context.Context, providerID string, accountKey *string, fn func(current *Credential) (*Credential, error)) (*Credential, error) {
	return s.modifyLocal(ctx, providerID, accountKey, true, fn)
}

// modifyLocal serializes read-modify-write for one account.
//
// The callback is synchronous, so no SQLite transaction spans it. Serialization instead comes from
// two layers: callbacks for the same provider queue in this process, and - because an OAuth refresh
// spends a single-use refresh token - the callback holds OMP's refresh lease on the credential row
// while it runs, so a sibling process waits instead of replaying the token.
//
// Writing differs by intent:
//   - An explicit accountKey updates exactly that enabled row (never a new row, never another
//     identity) and leaves the provider's selection alone. A missing row stays missing: a refresh
//     must not resurrect a logged-out account.
//   - Without a key, the write is a login upsert (saving a credential makes it the default), but a
//     refresh of an existing account keeps that row's identity_key and id so OMP keeps matching
//     blocks, leases, and org-scoped identity keys derived from the credential claims.
func (s *SQLiteAuthStorage) modifyLocal(ctx context.Context, providerID string, accountKey *string, explicit bool, fn func(current *Credential) (*Credential, error)) (*Credential, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if err := s.assertWritable(); err != nil {
		return nil, err
	}
	release, err := s.lockProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	defer release()

	// An explicit account that is gone (logged out, disabled) has nothing to write, and calling fn
	// would spend an OAuth refresh token on a row the user removed.
	var row *credentialRow
	if explicit {
		row, err = s.exactRow(ctx, providerID, accountKey)
	} else {
		row, err = s.rowFor(ctx, providerID, nil)
	}
	if err != nil {
		return nil, err
	}
	if explicit && row == nil {
		return nil, nil
	}
	current := resolveRowCredential(row)
	if row == nil || row.CredentialType != string(CredentialOAuth) {
		next, err := fn(current)
		if err != nil {
			return nil, err
		}
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		if next == nil {
			return current, nil
		}
		if row == nil {
			return s.upsertRow(ctx, providerID, *next, nil)
		}
		// An explicit account is written exactly as named; only a default write may treat a different
		// identity as a login and add or select an account.
		if !explicit && startsAnotherAccount(row, *next) {
			return s.upsertRow(ctx, providerID, *next, row)
		}
		// A row that vanished under us was logged out: report no credential, not the stale one.
		written, err := s.writeInPlace(ctx, row, *next, nil)
		if err != nil {
			return nil, err
		}
		if !written {
			return nil, nil
		}
		return next, nil
	}

	lease, leasing, err := s.acquireRefreshLease(ctx, row)
	if err != nil {
		return nil, err
	}
	// A live peer holds the lease past our wait budget: its exchange may already have consumed the
	// stored refresh token, so running fn would replay a spent token. Hand back the peer's rotation
	// when it landed, otherwise fail explicitly so the caller can fall back. Without a lease table
	// (an older OMP file) cross-process coordination is unavailable and in-process serialization is
	// all there is.
	if leasing && lease == nil {
		return s.resultAfterLeaseLoss(ctx, providerID, row, "lease timeout")
	}
	stopRenewal := make(chan struct{})
	if lease != nil {
		go s.keepRefreshLease(*lease, stopRenewal)
	}
	defer func() {
		close(stopRenewal)
		if lease != nil {
			s.releaseRefreshLease(*lease)
		}
	}()

	// Re-read under the lease: the holder we waited for may just have rotated this row, and a logout
	// may have removed it. Either way fn must see what is stored now.
	leased, err := s.rowByID(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	if leased == nil || leased.DisabledCause.Valid {
		return nil, nil
	}
	next, err := fn(resolveRowCredential(leased))
	if err != nil {
		// ompig-slim addition: only a definitive rejection from the JSON importer
		// retires secrets, under the same lease and compare-and-swap as refresh.
		if providerID == codexJSONProviderID && errors.Is(err, errCodexJSONExpired) {
			if cleanupErr := s.retireCodexJSON(ctx, leased, lease); cleanupErr != nil {
				return nil, fmt.Errorf("Codex JSON cleanup failed: %w", cleanupErr)
			}
		}
		return nil, err
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if next == nil {
		latest, err := s.rowByID(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		if latest != nil && !latest.DisabledCause.Valid {
			return resolveRowCredential(latest), nil
		}
		return nil, nil
	}
	// Ownership is checked by the fenced write below, not by this advisory renewal: a process pause
	// can outlive any renew-then-write sequence.
	if !explicit && startsAnotherAccount(leased, *next) {
		return s.upsertRow(ctx, providerID, *next, leased)
	}
	written, err := s.writeInPlace(ctx, leased, *next, lease)
	if err != nil {
		return nil, err
	}
	if written {
		return next, nil
	}
	// The fence failed: a peer took the lease, or a logout landed while fn ran. Never return the
	// pre-refresh token as if the refresh succeeded.
	return s.resultAfterLeaseLoss(ctx, providerID, leased, "lease lost")
}

// resultAfterLeaseLoss handles a refresh that could not publish its result because a peer holds the
// lease or removed the row. It returns whatever is stored now - a peer's rotation is the truth - or
// nil when the account is gone, and fails when the stored data is still the caller's own: then no
// refresh happened and its token must not be reported as valid.
func (s *SQLiteAuthStorage) resultAfterLeaseLoss(ctx context.Context, providerID string, expected *credentialRow, failure string) (*Credential, error) {
	latest, err := s.rowByID(ctx, expected.ID)
	if err != nil {
		return nil, err
	}
	if latest == nil || latest.DisabledCause.Valid {
		return nil, nil
	}
	if latest.Data != expected.Data {
		return resolveRowCredential(latest), nil
	}
	return nil, fmt.Errorf("OAuth refresh %s for provider %s", failure, providerID)
}

// startsAnotherAccount reports whether next must become its own row instead of refreshing row. A
// rotated OAuth token can lose the claims its identity key was built from, so a missing identity is
// a refresh; a different account identity - or a different credential type, which no refresh
// produces - is a login and follows login upsert rules.
//
// The comparison is between keys derived from the two credentials, never between a derived key and
// the stored identity_key: OMP stores email:/account:/project:/org:-qualified forms that
// deriveAccountKey does not produce, so comparing representations would read an ordinary refresh as
// a new account and duplicate the row.
func startsAnotherAccount(row *credentialRow, next Credential) bool {
	if string(next.Type) != row.CredentialType {
		return true
	}
	stored := rowCredential(row)
	if stored == nil {
		return false
	}
	before := deriveAccountKey(*stored)
	after := deriveAccountKey(next)
	return before != nil && after != nil && *before != *after
}

// writeInPlace rewrites row keeping its id and stored identity key. A refresh is never a login, so
// it must not touch the provider's selection: another request may already be authenticating with a
// different account.
//
// With a lease the update is fenced on that lease still being ours and unexpired in the same
// statement, so a peer takeover between renewal and write cannot be overwritten.
func (s *SQLiteAuthStorage) writeInPlace(ctx context.Context, row *credentialRow, credential Credential, lease *refreshLease) (bool, error) {
	data, err := encodeCredentialData(credential)
	if err != nil {
		return false, err
	}
	var (
		result  sql.Result
		execErr error
	)
	if lease != nil {
		result, execErr = s.db.ExecContext(ctx, `UPDATE auth_credentials
			SET credential_type = ?, data = ?, updated_at = `+sqliteNow+`
			WHERE id = ? AND disabled_cause IS NULL
			AND EXISTS (
				SELECT 1 FROM auth_credential_refresh_leases
				WHERE credential_id = ? AND owner = ? AND expires_at_ms > ?
			)`, string(credential.Type), data, row.ID, lease.id, lease.owner, time.Now().UnixMilli())
	} else {
		result, execErr = s.db.ExecContext(ctx, `UPDATE auth_credentials
			SET credential_type = ?, data = ?, updated_at = `+sqliteNow+`
			WHERE id = ? AND disabled_cause IS NULL`, string(credential.Type), data, row.ID)
	}
	if execErr != nil {
		return false, execErr
	}
	changes, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return changes > 0, nil
}

// upsertRow performs a login-style write under the credential's own identity: replace the row with
// that identity (reviving a tombstone, as logging in again should) or insert one, and select it.
// replacing is the row the callback started from, reused when the credential arrives without the
// claims its identity key was derived from.
func (s *SQLiteAuthStorage) upsertRow(ctx context.Context, providerID string, credential Credential, replacing *credentialRow) (*Credential, error) {
	derived := deriveAccountKey(credential)
	var accountKey *string
	if derived != nil {
		accountKey = derived
	} else if replacing != nil && replacing.IdentityKey.Valid {
		accountKey = new(replacing.IdentityKey.String)
	}
	data, err := encodeCredentialData(credential)
	if err != nil {
		return nil, err
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		// OMP replaces an API key with an OAuth login instead of holding both: a provider has one
		// credential type at a time, so the older key must not stay enabled as OMP's default.
		if credential.Type == CredentialOAuth {
			if _, err := tx.ExecContext(ctx, `UPDATE auth_credentials
				SET disabled_cause = ?, updated_at = `+sqliteNow+`
				WHERE provider = ? AND credential_type = 'api_key' AND disabled_cause IS NULL`,
				replacedByOAuthCause, providerID); err != nil {
				return err
			}
		}
		existing := int64(0)
		exists := false
		if replacing != nil && !startsAnotherAccount(replacing, credential) {
			existing, exists = replacing.ID, true
		} else {
			id, ok, err := rowIDByAccountKey(ctx, tx, providerID, accountKey)
			if err != nil {
				return err
			}
			if ok {
				existing, exists = id, true
			} else if same, ok, err := sameAccountRow(ctx, tx, providerID, credential); err != nil {
				return err
			} else if ok {
				existing, exists = same, true
			}
		}
		var rowID int64
		if exists {
			if _, err := tx.ExecContext(ctx, `UPDATE auth_credentials
				SET credential_type = ?, data = ?, disabled_cause = NULL, updated_at = `+sqliteNow+`
				WHERE id = ?`, string(credential.Type), data, existing); err != nil {
				return err
			}
			rowID = existing
		} else {
			var identity any
			if accountKey != nil {
				identity = *accountKey
			}
			if err := tx.QueryRowContext(ctx, `INSERT INTO auth_credentials (provider, credential_type, data, identity_key)
				VALUES (?, ?, ?, ?) RETURNING id`,
				providerID, string(credential.Type), data, identity).Scan(&rowID); err != nil {
				return err
			}
		}
		return s.setSelected(ctx, tx, providerID, rowID)
	})
	if err != nil {
		return nil, err
	}
	return &credential, nil
}

// sameAccountRow returns a stored row (enabled or tombstoned) of providerID that holds the same
// account as credential, compared through derived keys so OMP's qualified identity_key forms
// (email:, account:, project:, org:) still match. Keeps a login from adding a second row for an
// account OMP already stored under a different key representation.
func sameAccountRow(ctx context.Context, q querier, providerID string, credential Credential) (int64, bool, error) {
	wanted := deriveAccountKey(credential)
	if wanted == nil {
		return 0, false, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT `+rowColumns+`
		FROM auth_credentials WHERE provider = ? ORDER BY id ASC`, providerID)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		row, err := scanCredentialRow(rows)
		if err != nil {
			return 0, false, err
		}
		stored := rowCredential(&row)
		if stored == nil {
			continue
		}
		if derived := deriveAccountKey(*stored); derived != nil && *derived == *wanted {
			return row.ID, true, nil
		}
	}
	return 0, false, rows.Err()
}

// hasLeaseTable reports whether this database has OMP's refresh lease table.
func (s *SQLiteAuthStorage) hasLeaseTable(ctx context.Context) bool {
	present, err := s.hasTable(ctx, "auth_credential_refresh_leases")
	return err == nil && present
}

// acquireRefreshLease takes OMP's refresh lease on a row, so only one process exchanges its
// single-use refresh token. It waits for a live holder instead of replaying the token, and gives up
// when the caller's own wait budget runs out.
//
// Returns a nil lease and a false leasing flag when this database has no lease table (an older OMP
// file): cross-process coordination is then unavailable and the caller proceeds on in-process
// serialization alone. A true flag with a nil lease means the wait budget ran out.
func (s *SQLiteAuthStorage) acquireRefreshLease(ctx context.Context, row *credentialRow) (*refreshLease, bool, error) {
	// An existing OMP database written before leases existed has no table to take a lease in.
	if !s.hasLeaseTable(ctx) {
		return nil, false, nil
	}
	owner := uuid.NewString()
	deadline := time.Now().Add(refreshLeaseWaitMaxMs * time.Millisecond)
	for {
		if err := context.Cause(ctx); err != nil {
			return nil, true, err
		}
		now := time.Now().UnixMilli()
		result, err := s.db.ExecContext(ctx, `INSERT INTO auth_credential_refresh_leases
			(credential_id, owner, expires_at_ms, updated_at)
			VALUES (?, ?, ?, `+sqliteNow+`)
			ON CONFLICT(credential_id) DO UPDATE SET
				owner = excluded.owner,
				expires_at_ms = excluded.expires_at_ms,
				updated_at = excluded.updated_at
			WHERE auth_credential_refresh_leases.expires_at_ms <= ?`,
			row.ID, owner, now+refreshLeaseTTLMs, now)
		if err != nil {
			return nil, true, err
		}
		if changes, err := result.RowsAffected(); err != nil {
			return nil, true, err
		} else if changes > 0 {
			return &refreshLease{id: row.ID, owner: owner}, true, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, true, nil
		}
		var expiresAt int64
		err = s.db.QueryRowContext(ctx, "SELECT expires_at_ms FROM auth_credential_refresh_leases WHERE credential_id = ?", row.ID).Scan(&expiresAt)
		waitMs := int64(refreshLeasePollMs)
		if err == nil {
			waitMs = expiresAt - time.Now().UnixMilli()
			if waitMs < refreshLeasePollMs {
				waitMs = refreshLeasePollMs
			}
			if waitMs > refreshLeasePollMaxMs {
				waitMs = refreshLeasePollMaxMs
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, true, err
		}
		wait := time.Duration(waitMs) * time.Millisecond
		if wait > remaining {
			wait = remaining
		} else if wait < 0 {
			wait = 0
		}
		if err := leaseSleep(ctx, wait); err != nil {
			return nil, true, err
		}
	}
}

// renewRefreshLease extends our lease; false means another process took it and its write must not
// be overwritten.
func (s *SQLiteAuthStorage) renewRefreshLease(lease refreshLease) (bool, error) {
	result, err := s.db.ExecContext(context.Background(), `UPDATE auth_credential_refresh_leases
		SET expires_at_ms = ?, updated_at = `+sqliteNow+`
		WHERE credential_id = ? AND owner = ?`, time.Now().UnixMilli()+refreshLeaseTTLMs, lease.id, lease.owner)
	if err != nil {
		return false, err
	}
	changes, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return changes > 0, nil
}

// keepRefreshLease keeps a long exchange's lease alive so a peer waits instead of declaring it
// stale. Losing ownership is not tracked here: the fenced write already refuses to overwrite the
// new holder.
func (s *SQLiteAuthStorage) keepRefreshLease(lease refreshLease, stop <-chan struct{}) {
	ticker := time.NewTicker(refreshLeaseRenewMs * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if renewed, err := s.renewRefreshLease(lease); err != nil || !renewed {
				return
			}
		}
	}
}

func (s *SQLiteAuthStorage) releaseRefreshLease(lease refreshLease) {
	_, _ = s.db.ExecContext(context.Background(),
		"DELETE FROM auth_credential_refresh_leases WHERE credential_id = ? AND owner = ?", lease.id, lease.owner)
}

// GetAccountBlockedUntil returns the latest live block deadline among scopes for one account,
// including OMP's unscoped (\"\") block, or nil when it has none.
func (s *SQLiteAuthStorage) GetAccountBlockedUntil(ctx context.Context, providerID string, accountKey *string, scopes []string) (*int64, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	present, err := s.hasTable(ctx, "auth_credential_blocks")
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}
	row, err := s.exactRow(ctx, providerID, accountKey)
	if err != nil || row == nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT block_scope, blocked_until_ms FROM auth_credential_blocks
		WHERE credential_id = ? AND provider_key = ? AND blocked_until_ms > ?`,
		row.ID, providerKey(row), time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	deadlines := map[string]int64{}
	for rows.Next() {
		var scope string
		var until int64
		if err := rows.Scan(&scope, &until); err != nil {
			return nil, err
		}
		deadlines[scope] = until
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// OMP's unscoped row applies to every scope, so it is always consulted.
	var latest *int64
	for _, scope := range append([]string{""}, scopes...) {
		deadline, ok := deadlines[scope]
		if !ok {
			continue
		}
		if latest == nil || deadline > *latest {
			value := deadline
			latest = &value
		}
	}
	return latest, nil
}

// BlockAccount records a block on one account. Later writes never shorten an existing deadline.
func (s *SQLiteAuthStorage) BlockAccount(ctx context.Context, providerID string, accountKey *string, scope string, untilMs int64) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := s.assertWritable(); err != nil {
		return err
	}
	present, err := s.hasTable(ctx, "auth_credential_blocks")
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	row, err := s.exactRow(ctx, providerID, accountKey)
	if err != nil || row == nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO auth_credential_blocks
		(credential_id, provider_key, block_scope, blocked_until_ms, updated_at)
		VALUES (?, ?, ?, ?, `+sqliteNow+`)
		ON CONFLICT(credential_id, provider_key, block_scope) DO UPDATE SET
			blocked_until_ms = MAX(blocked_until_ms, excluded.blocked_until_ms),
			updated_at = excluded.updated_at`,
		row.ID, providerKey(row), scope, untilMs)
	return err
}

// ClearAccountBlocks drops one account's blocks: every scope when scopes is empty, else exactly
// those scopes. An empty scope string is the unscoped ("") row, which applies to every request.
//
// A reset that covered only some windows must pass those windows' scopes, not the empty list:
// clearing every scope would also lift an unrelated auth or model-policy block.
func (s *SQLiteAuthStorage) ClearAccountBlocks(ctx context.Context, providerID string, accountKey *string, scopes []string) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := s.assertWritable(); err != nil {
		return err
	}
	present, err := s.hasTable(ctx, "auth_credential_blocks")
	if err != nil || !present {
		return err
	}
	row, err := s.exactRow(ctx, providerID, accountKey)
	if err != nil || row == nil {
		return err
	}
	if len(scopes) == 0 {
		_, err = s.db.ExecContext(ctx, `DELETE FROM auth_credential_blocks
			WHERE credential_id = ? AND provider_key = ?`, row.ID, providerKey(row))
		return err
	}
	for _, scope := range scopes {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM auth_credential_blocks
			WHERE credential_id = ? AND provider_key = ? AND block_scope = ?`, row.ID, providerKey(row), scope); err != nil {
			return err
		}
	}
	return nil
}

func providerKey(row *credentialRow) string {
	return row.Provider + ":" + row.CredentialType
}

// Delete scrubs and tombstones every enabled credential of a provider and drops its selection.
func (s *SQLiteAuthStorage) Delete(ctx context.Context, providerID string) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := s.assertWritable(); err != nil {
		return err
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE auth_credentials
			SET data = '{}', disabled_cause = ?, updated_at = `+sqliteNow+`
			WHERE provider = ? AND disabled_cause IS NULL`, logoutCause, providerID); err != nil {
			return err
		}
		return s.clearSelection(ctx, tx, providerID)
	})
}

// clearSelection drops a provider's recorded selection; Read then falls back to OMP's default.
func (s *SQLiteAuthStorage) clearSelection(ctx context.Context, q querier, providerID string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM cache WHERE key = ?", selectionKey(providerID))
	return err
}

// lockProvider serializes callbacks for one provider in this process, honoring ctx while waiting.
func (s *SQLiteAuthStorage) lockProvider(ctx context.Context, providerID string) (func(), error) {
	s.queuesMu.Lock()
	queue := s.queues[providerID]
	if queue == nil {
		queue = make(chan struct{}, 1)
		if s.queues == nil {
			s.queues = map[string]chan struct{}{}
		}
		s.queues[providerID] = queue
	}
	s.queuesMu.Unlock()
	select {
	case queue <- struct{}{}:
		return func() { <-queue }, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

// leaseSleep waits for duration or the context's cancellation, preserving the context's cause.
func leaseSleep(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return context.Cause(ctx)
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// ─── Credential payloads ─────────────────────────────────────────────────────

// deriveAccountKey derives the account key for a credential: stable for the account it belongs to,
// so storing the same account twice replaces its row instead of adding one.
//
// OAuth identity comes from the token's own claims, in the order accountId, email, projectId,
// mirroring OMP's oauthAccountKey, with orgId appended when present so two organizations of one
// account stay separate. API keys have no identity claim, so the key is an env hint or a digest of
// the key itself; hashing keeps a secret out of a column the app reads for display.
func deriveAccountKey(credential Credential) *string {
	if credential.Type == CredentialOAuth {
		base := firstNonEmptyClaim(
			jsstring.Trim(credential.AccountID),
			credentialClaim(credential, "email"),
			jsstring.Trim(credential.ProjectID),
		)
		if base == "" {
			return nil
		}
		if org := credentialClaim(credential, "orgId"); org != "" {
			key := base + "|org:" + org
			return &key
		}
		return &base
	}
	if hint := envAccountHint(credential); hint != "" {
		key := "env:" + hint
		return &key
	}
	key := jsstring.Trim(credential.Key)
	if key == "" {
		return nil
	}
	var derived string
	if len([]rune(key)) <= 8 {
		derived = "key:" + key
	} else {
		derived = "key:" + hashSecret(key)
	}
	return &derived
}

// accountEnvHints are provider env values that name an account, so two keys of one provider stay
// distinguishable.
var accountEnvHints = []string{"ANTHROPIC_ACCOUNT_ID", "CLOUDFLARE_ACCOUNT_ID", "GOOGLE_CLOUD_PROJECT", "OPENAI_ORG_ID"}

func envAccountHint(credential Credential) string {
	for _, name := range accountEnvHints {
		if value, ok := credential.Env[name]; ok {
			if trimmed := jsstring.Trim(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// hashSecret is a short, non-reversible tag for a secret, so two keys of one provider stay
// distinguishable. It reproduces the JavaScript double-FNV loop over UTF-16 code units.
func hashSecret(secret string) string {
	first := uint32(0x811c9dc5)
	second := uint32(0x01000193)
	for _, unit := range utf16.Encode([]rune(secret)) {
		// Math.imul: the 32-bit product of the truncated operands.
		first = uint32(int32(first^uint32(unit)) * int32(0x01000193))
		second = uint32(int32(second+uint32(unit)) * -2048144789)
	}
	var encoded [16]byte
	hex.Encode(encoded[:8], []byte{
		byte(first >> 24), byte(first >> 16), byte(first >> 8), byte(first),
	})
	hex.Encode(encoded[8:], []byte{
		byte(second >> 24), byte(second >> 16), byte(second >> 8), byte(second),
	})
	return string(encoded[:])
}

func firstNonEmptyClaim(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// credentialClaim reads a non-secret string claim a provider stored in the credential's Extra
// fields (email, orgId and provider metadata live there, not in typed columns).
func credentialClaim(credential Credential, field string) string {
	raw, ok := credential.Extra[field]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return jsstring.Trim(value)
}

// accountLabel is the display label for an account, from the credential's own non-secret claims.
func accountLabel(credential Credential) string {
	if credential.Type == CredentialOAuth {
		return firstNonEmptyClaim(
			credentialClaim(credential, "email"),
			jsstring.Trim(credential.AccountID),
			jsstring.Trim(credential.ProjectID),
		)
	}
	return envAccountHint(credential)
}

// decodeCredentialData parses a stored row's payload in the shape OMP writes.
func decodeCredentialData(credentialType, data string) *Credential {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &fields); err != nil || fields == nil {
		return nil
	}
	switch credentialType {
	case string(CredentialAPIKey):
		// Pi's row decoder keeps the key and any provider env: the env carries non-secret provider
		// settings (Cloudflare account, Google project) that request auth reads back.
		raw, ok := fields["key"]
		if !ok {
			return nil
		}
		var key string
		if err := json.Unmarshal(raw, &key); err != nil {
			return nil
		}
		credential := &Credential{Type: CredentialAPIKey, Key: key}
		if raw, ok := fields["env"]; ok {
			var env map[string]string
			if json.Unmarshal(raw, &env) == nil && len(env) > 0 {
				credential.Env = env
			}
		}
		return credential
	case string(CredentialOAuth):
		// `{ type: "oauth", ...data }`: a type the row carries itself wins, as a spread would.
		if _, ok := fields["type"]; !ok {
			fields["type"] = json.RawMessage(`"oauth"`)
		}
		encoded, err := json.Marshal(fields)
		if err != nil {
			return nil
		}
		var credential Credential
		if err := json.Unmarshal(encoded, &credential); err != nil {
			return nil
		}
		credential.Type = CredentialOAuth
		return &credential
	default:
		return nil
	}
}

// encodeCredentialData serializes in the shape OMP reads: `{ key }` plus any provider env for API
// keys, claims for OAuth (without the discriminator, which the column holds).
func encodeCredentialData(credential Credential) (string, error) {
	if credential.Type == CredentialAPIKey {
		fields := map[string]json.RawMessage{}
		raw, err := json.Marshal(credential.Key)
		if err != nil {
			return "", err
		}
		fields["key"] = raw
		if len(credential.Env) > 0 {
			raw, err = json.Marshal(credential.Env)
			if err != nil {
				return "", err
			}
			fields["env"] = raw
		}
		keys := []string{"key"}
		if _, ok := fields["env"]; ok {
			keys = append(keys, "env")
		}
		encoded, err := writeOrderedJSONObject(keys, fields)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return "", err
	}
	// Pi stores `{ ...credential }` without the discriminator, keeping the credential's own member
	// order: OMP writes that object and reads it back by field name.
	keys, fields, err := jsonObjectEntries(encoded)
	if err != nil {
		return "", err
	}
	delete(fields, "type")
	keys = slices.DeleteFunc(keys, func(key string) bool { return key == "type" })
	withoutType, err := writeOrderedJSONObject(keys, fields)
	if err != nil {
		return "", err
	}
	return string(withoutType), nil
}

func infoCredentialType(credentialType string) CredentialType {
	if credentialType == string(CredentialOAuth) {
		return CredentialOAuth
	}
	return CredentialAPIKey
}

func accountInfo(row *credentialRow, active bool) CredentialAccountInfo {
	info := CredentialAccountInfo{
		ProviderID: row.Provider,
		Type:       infoCredentialType(row.CredentialType),
		Active:     active,
		CreatedAt:  row.CreatedAt,
	}
	if row.IdentityKey.Valid {
		key := row.IdentityKey.String
		info.AccountKey = &key
	}
	if credential := rowCredential(row); credential != nil {
		info.Label = accountLabel(*credential)
	}
	return info
}

// ─── Store locations ─────────────────────────────────────────────────────────

// SQLiteAuthPath is the multi-account credential store inside an agent directory.
func SQLiteAuthPath(agentDir string) string { return filepath.Join(agentDir, "agent.db") }

// LegacyAuthJSONPath is the single-credential file the store replaces. Pi still reports it in
// login status text (getAuthPath) and migration reads it.
func LegacyAuthJSONPath(agentDir string) string { return filepath.Join(agentDir, "auth.json") }

// OpenCredentialStore discovers the application-owned credential store for this
// profile: the configured broker, otherwise local multi-account SQLite with its
// one-time auth.json migration. A configured broker never falls back to SQLite.
func OpenCredentialStore(agentDir string) (ApplicationCredentialStore, error) {
	if CredentialStoreFactory != nil {
		return CredentialStoreFactory(agentDir)
	}
	return OpenLocalCredentialStore(agentDir)
}

// OpenLocalCredentialStore opens SQLite explicitly, bypassing remote discovery.
// Only vault ownership and an explicit local-to-broker migration should use it.
func OpenLocalCredentialStore(agentDir string) (*SQLiteAuthStorage, error) {
	store, err := OpenSQLiteAuthStorage(SQLiteAuthPath(agentDir))
	if err != nil {
		return nil, err
	}
	MigrateAuthJSONToSQLite(store, agentDir)
	return store, nil
}

// OpenReadOnlyCredentialStore discovers broker credentials when configured;
// otherwise it inspects local SQLite or auth.json without creation/migration.
// The returned store is used for inspection; callers must not mutate it.
func OpenReadOnlyCredentialStore(agentDir string) (CredentialStore, error) {
	if CredentialStoreFactory != nil {
		return CredentialStoreFactory(agentDir)
	}
	path := SQLiteAuthPath(agentDir)
	if _, err := os.Stat(path); err == nil {
		return OpenReadOnlySQLiteAuthStorage(path)
	}
	return NewReadOnlyAuthStorage(filepath.Join(agentDir, "auth.json")), nil
}
