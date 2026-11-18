package ai

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func sessionCachePrefix(providerID string) string {
	return fmt.Sprintf("omp-slim:auth-route:%x:", sha256.Sum256([]byte(providerID)))
}

func sessionCacheKey(providerID string, route AccountSessionRoute) string {
	// Usage changes must not alter the durable session/model pin identity.
	identity, _ := json.Marshal(struct {
		SessionID string `json:"sessionId"`
		ModelID   string `json:"modelId"`
	}{SessionID: route.SessionID, ModelID: route.ModelID})
	return fmt.Sprintf("%spin:%x", sessionCachePrefix(providerID), sha256.Sum256(identity))
}

func readRouteID(ctx context.Context, q querier, key string) (int64, error) {
	var value string
	err := q.QueryRowContext(ctx, "SELECT value FROM cache WHERE key = ? AND expires_at > ?", key, time.Now().Unix()).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(value, 10, 64)
}

func writeRouteID(ctx context.Context, q querier, key string, id int64) error {
	_, err := q.ExecContext(ctx, `INSERT INTO cache (key, value, expires_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at`,
		key, strconv.FormatInt(id, 10), time.Now().Unix()+selectionCacheLifetime)
	return err
}

func validateSessionRoute(providerID string, route AccountSessionRoute) error {
	if providerID == "" || route.SessionID == "" || route.ModelID == "" {
		return errors.New("invalid session route")
	}
	return nil
}

func sessionRows(ctx context.Context, q querier, providerID string, route AccountSessionRoute, selection *AccountSelection) ([]credentialRow, error) {
	scopes := append([]string{""}, AccountRequestBlockScopes(providerID, route.ModelID)...)
	args := []any{providerID, time.Now().UnixMilli()}
	for _, scope := range scopes {
		args = append(args, scope)
	}
	query := `SELECT ` + rowColumns + ` FROM auth_credentials c
		WHERE provider = ? AND disabled_cause IS NULL AND NOT EXISTS (
			SELECT 1 FROM auth_credential_blocks b WHERE b.credential_id = c.id
			AND b.provider_key = c.provider || ':' || c.credential_type AND b.blocked_until_ms > ?
			AND b.block_scope IN (` + strings.TrimSuffix(strings.Repeat("?,", len(scopes)), ",") + `))`
	if selection != nil {
		query += " AND identity_key IS ?"
		args = append(args, selection.AccountKey)
	}
	query += " ORDER BY id ASC"
	if selection != nil {
		query += " LIMIT 1"
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []credentialRow
	for rows.Next() {
		row, err := scanCredentialRow(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, row)
	}
	return accounts, rows.Err()
}

func usageSessionCursorKey(providerID string, rank accountPlanRank) string {
	return sessionCachePrefix(providerID) + "usage-cursor:" + strconv.Itoa(int(rank))
}

// ResolveSessionAccount keeps previews read-only and serializes reservations across processes.
func (s *SQLiteAuthStorage) ResolveSessionAccount(ctx context.Context, providerID string, route AccountSessionRoute, reserve bool) (*AccountSelection, error) {
	if err := validateSessionRoute(providerID, route); err != nil {
		return nil, err
	}
	var selected *AccountSelection
	resolve := func(q querier) error {
		rows, err := sessionRows(ctx, q, providerID, route, nil)
		if err != nil {
			return err
		}
		best := accountPlanIneligible
		var candidates []credentialRow
		usageByRow := make(map[int64]*AccountSessionUsage, len(rows))
		usageAware := false
		now := time.Now()
		for _, row := range rows {
			credential := rowCredential(&row)
			if credential == nil {
				continue
			}
			var accountKey *string
			if row.IdentityKey.Valid {
				accountKey = &row.IdentityKey.String
			}
			usage := accountSessionUsageFor(route, accountKey, now)
			if accountSessionUsageExhausted(usage) {
				continue
			}
			rank := accountPlanPriorityWithUsage(providerID, route.ModelID, *credential, usage)
			if rank == accountPlanIneligible || rank > best {
				continue
			}
			if rank < best {
				best, candidates, usageAware = rank, nil, false
			}
			candidates = append(candidates, row)
			if usage != nil {
				usageByRow[row.ID] = usage
				usageAware = true
			}
		}
		if len(candidates) == 0 {
			return nil
		}
		pinKey := sessionCacheKey(providerID, route)
		pinned, err := readRouteID(ctx, q, pinKey)
		if err != nil {
			return err
		}
		choose := func(row credentialRow) {
			selected = &AccountSelection{AccountKeySet: true}
			if row.IdentityKey.Valid {
				selected.AccountKey = new(row.IdentityKey.String)
			}
		}
		for _, row := range candidates {
			if row.ID == pinned {
				choose(row)
				return nil
			}
		}
		cursorKey := sessionCachePrefix(providerID) + "cursor:" + strconv.Itoa(int(best))
		if usageAware {
			cursorKey = usageSessionCursorKey(providerID, best)
			preferred := candidates[:0]
			bestRemaining := -1.0
			for _, row := range candidates {
				remaining := shortestWindowRemaining(usageByRow[row.ID])
				if remaining > bestRemaining {
					bestRemaining = remaining
					preferred = preferred[:0]
				}
				if remaining == bestRemaining {
					preferred = append(preferred, row)
				}
			}
			candidates = preferred
		}
		cursor, err := readRouteID(ctx, q, cursorKey)
		if err != nil {
			return err
		}
		chosen := candidates[0]
		if cursor == 0 {
			active, _, err := selectedIDOn(ctx, q, providerID)
			if err != nil {
				return err
			}
			for _, row := range candidates {
				if row.ID == active {
					chosen = row
					break
				}
			}
		} else {
			for _, row := range candidates {
				if row.ID > cursor {
					chosen = row
					break
				}
			}
		}
		choose(chosen)
		if !reserve {
			return nil
		}
		if err := writeRouteID(ctx, q, pinKey, chosen.ID); err != nil {
			return err
		}
		return writeRouteID(ctx, q, cursorKey, chosen.ID)
	}
	if !reserve {
		err := resolve(s.db)
		return selected, err
	}
	if err := s.assertWritable(); err != nil {
		return nil, err
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error { return resolve(tx) })
	return selected, err
}

// SetSessionAccount persists a fallback before transport can emit any output.
func (s *SQLiteAuthStorage) SetSessionAccount(ctx context.Context, providerID string, route AccountSessionRoute, key *string) error {
	if err := validateSessionRoute(providerID, route); err != nil {
		return err
	}
	if err := s.assertWritable(); err != nil {
		return err
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := sessionRows(ctx, tx, providerID, route, &AccountSelection{AccountKey: key, AccountKeySet: true})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("session account is no longer available")
		}
		row := rows[0]
		credential := rowCredential(&row)
		if credential == nil {
			return errors.New("session account is no longer available")
		}
		var accountKey *string
		if row.IdentityKey.Valid {
			accountKey = &row.IdentityKey.String
		}
		usage := accountSessionUsageFor(route, accountKey, time.Now())
		if accountSessionUsageExhausted(usage) || accountPlanPriorityWithUsage(providerID, route.ModelID, *credential, usage) == accountPlanIneligible {
			return errors.New("session account is no longer available")
		}
		return writeRouteID(ctx, tx, sessionCacheKey(providerID, route), row.ID)
	})
}
