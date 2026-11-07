package ai

import (
	"context"
	"database/sql"
	"strconv"
	"time"
)

// retireCodexJSON scrubs a permanently failed imported credential and its blocks
// atomically. Keep OMP's tombstone/id, but no bearer or refresh material. The
// lease and original data fence protect a concurrent re-import or peer refresh;
// a stale failure must never retire a newer credential. Selection is removed
// only if it still points at this row. This does not promise forensic erasure of
// SQLite WAL pages or external backups, only removal from live database state.
func (s *SQLiteAuthStorage) retireCodexJSON(ctx context.Context, row *credentialRow, lease *refreshLease) error {
	if row.Provider != codexJSONProviderID || lease == nil {
		return nil
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE auth_credentials
   SET data = '{}', disabled_cause = 'expired Codex JSON', updated_at = `+sqliteNow+`
   WHERE id = ? AND provider = ? AND data = ? AND disabled_cause IS NULL
   AND EXISTS (SELECT 1 FROM auth_credential_refresh_leases
    WHERE credential_id = ? AND owner = ? AND expires_at_ms > ?)`,
			row.ID, codexJSONProviderID, row.Data, lease.id, lease.owner, time.Now().UnixMilli())
		if err != nil {
			return err
		}
		changes, err := result.RowsAffected()
		if err != nil || changes == 0 {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM auth_credential_blocks WHERE credential_id = ?", row.ID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM cache WHERE key = ? AND value = ?", selectionKey(row.Provider), strconv.FormatInt(row.ID, 10))
		return err
	})
}
