package ai

// One-time move from the single-credential auth.json to the multi-account SQLite store.
//
// Ports packages/coding-agent/src/core/auth-json-migration.ts.
//
// auth.json holds one credential per provider, keyed by provider id. The SQLite store
// (sqlite_auth_storage.go) holds any number per provider, keyed by account. Importing the file is
// therefore lossless: each entry becomes that provider's first, active account.
//
// The file is renamed to auth.json.migrated rather than deleted, so a downgrade can restore it and a
// failed import leaves the original in place. Migration runs only when the database has no rows, so
// a store emptied by an explicit logout is not silently repopulated from a stale file.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/MichaelKinsy/PiG/internal/jsonparse"
	"github.com/MichaelKinsy/PiG/internal/text"
)

// AuthMigrationResult reports what a migration did.
type AuthMigrationResult struct {
	// MigratedProviders are the providers imported from the legacy file, in file order.
	MigratedProviders []string
	// ArchivedPath is the path the legacy file was renamed to, when a migration ran.
	ArchivedPath string
}

// MigrateAuthJSONToSQLite imports <agentDir>/auth.json into store when the store is empty, and
// renames the file to auth.json.migrated. It returns an empty result when there is nothing to do:
// no legacy file, or the store already holds credentials. A store that cannot be read or a file
// that cannot be parsed leaves both sides untouched.
func MigrateAuthJSONToSQLite(store *SQLiteAuthStorage, agentDir string) AuthMigrationResult {
	if store == nil || store.readOnly {
		return AuthMigrationResult{}
	}
	authPath := filepath.Join(agentDir, "auth.json")
	if _, err := os.Stat(authPath); err != nil {
		return AuthMigrationResult{}
	}
	// A store that already holds credentials keeps them; the legacy file is left untouched so the
	// user can inspect it instead of us guessing which side is authoritative.
	if count, err := store.AccountCount(); err != nil || count > 0 {
		return AuthMigrationResult{}
	}
	data, err := os.ReadFile(authPath)
	if err != nil {
		return AuthMigrationResult{}
	}
	// JSON.parse accepts a leading BOM after stripBom; the shared validator reports V8's SyntaxError.
	body := text.StripBomBytes(data)
	if err := jsonparse.Validate(body); err != nil {
		return AuthMigrationResult{}
	}
	order, values, err := objectEntries(body)
	if err != nil {
		return AuthMigrationResult{}
	}
	ctx := context.Background()
	migrated := []string{}
	for _, providerID := range order {
		credential, ok := legacyCredential(values[providerID])
		if !ok {
			continue
		}
		// saveAccount makes each imported entry the provider's active account, matching the file,
		// which could only describe one credential per provider.
		if err := store.saveAccount(ctx, providerID, deriveAccountKey(*credential), *credential); err != nil {
			return AuthMigrationResult{}
		}
		migrated = append(migrated, providerID)
	}
	if len(migrated) == 0 {
		return AuthMigrationResult{}
	}
	archivedPath := authPath + ".migrated"
	if err := os.Rename(authPath, archivedPath); err != nil {
		// The import already succeeded; a rename failure (read-only directory, Windows sharing
		// violation) must not roll it back. The next start sees a non-empty store and skips.
		return AuthMigrationResult{MigratedProviders: migrated}
	}
	return AuthMigrationResult{MigratedProviders: migrated, ArchivedPath: archivedPath}
}

// legacyCredential reads one imported auth.json entry. It decodes the whole entry, keeping
// provider-owned metadata, and reports false when the entry is not an api_key or oauth credential.
func legacyCredential(raw json.RawMessage) (*Credential, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, false
	}
	var kind CredentialType
	if rawKind, ok := fields["type"]; !ok || json.Unmarshal(rawKind, &kind) != nil {
		return nil, false
	}
	if kind != CredentialAPIKey && kind != CredentialOAuth {
		return nil, false
	}
	var credential Credential
	if err := json.Unmarshal(raw, &credential); err != nil {
		return nil, false
	}
	credential.Type = kind
	return &credential, true
}

// objectEntries reads a JSON object's keys at their first position and each key's last value, as
// JSON.parse keeps them.
func objectEntries(data []byte) ([]string, map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return nil, nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, nil, errors.New("legacy auth file is not a JSON object")
	}
	var keys []string
	values := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, nil, errors.New("legacy auth key is not a string")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, err
		}
		if _, seen := values[key]; !seen {
			keys = append(keys, key)
		}
		values[key] = value
	}
	return javascriptObjectKeyOrder(keys), values, nil
}
