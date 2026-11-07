package ai

import (
	"testing"
	"time"
)

// TestCodexJSONCleanupFence verifies the destructive write cannot retire a newer
// login, a peer's refresh lease, or another account's explicit selection.
func TestCodexJSONCleanupFence(t *testing.T) {
	for _, mode := range []string{"newer login", "lease takeover", "selected sibling", "selected self"} {
		t.Run(mode, func(t *testing.T) {
			store, _ := openTestStore(t)
			ctx := t.Context()
			old := oauthCredential("a", "", time.Now().Add(-time.Hour))
			key := deriveAccountKey(old)
			if _, err := store.SaveAccount(ctx, codexJSONProviderID, key, old); err != nil {
				t.Fatal(err)
			}
			row, err := store.exactRow(ctx, codexJSONProviderID, key)
			if err != nil {
				t.Fatal(err)
			}
			lease, _, err := store.acquireRefreshLease(ctx, row)
			if err != nil || lease == nil {
				t.Fatalf("lease: %v", err)
			}
			defer store.releaseRefreshLease(*lease)
			switch mode {
			case "newer login":
				newer := old
				newer.Access = "newer-fixture"
				newer.Expires = time.Now().Add(time.Hour).UnixMilli()
				if _, err := store.SaveAccount(ctx, codexJSONProviderID, key, newer); err != nil {
					t.Fatal(err)
				}
			case "lease takeover":
				if _, err := store.db.Exec("UPDATE auth_credential_refresh_leases SET owner = 'peer' WHERE credential_id = ?", row.ID); err != nil {
					t.Fatal(err)
				}
			case "selected sibling":
				sibling := oauthCredential("b", "", time.Now().Add(time.Hour))
				if _, err := store.SaveAccount(ctx, codexJSONProviderID, deriveAccountKey(sibling), sibling); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.retireCodexJSON(ctx, row, lease); err != nil {
				t.Fatal(err)
			}
			saved, err := store.ReadAccount(ctx, codexJSONProviderID, key)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "newer login":
				if saved == nil || saved.Access != "newer-fixture" {
					t.Fatal("stale rejection scrubbed new login")
				}
			case "lease takeover":
				if saved == nil {
					t.Fatal("stale lease scrubbed a peer's credential")
				}
			case "selected sibling":
				if saved != nil {
					t.Fatal("dead account not retired")
				}
				selected, err := store.Read(ctx, codexJSONProviderID)
				if err != nil || selected == nil || selected.AccountID != "b" {
					t.Fatal("cleanup changed sibling selection")
				}
				if _, present, err := store.selectedID(ctx, codexJSONProviderID); err != nil || !present {
					t.Fatal("sibling selection cache removed")
				}
			case "selected self":
				if saved != nil {
					t.Fatal("dead account not retired")
				}
				var count int
				if err := store.db.QueryRow("SELECT COUNT(*) FROM cache WHERE key = ?", selectionKey(codexJSONProviderID)).Scan(&count); err != nil || count != 0 {
					t.Fatalf("retired selection retained: %d %v", count, err)
				}
			}
		})
	}
}
