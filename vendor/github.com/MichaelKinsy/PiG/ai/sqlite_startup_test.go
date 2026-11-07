package ai

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestSQLiteFastSelectionRejectsForeignDisabledAndStaleRows(t *testing.T) {
	store, _ := openTestStore(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	future := time.Now().Add(time.Hour)
	for _, account := range []struct{ provider, id string }{{"fixture", "first"}, {"fixture", "second"}, {"other", "foreign"}} {
		credential := oauthCredential(account.id, account.id+"@example.com", future)
		if _, err := store.SaveAccount(ctx, account.provider, deriveAccountKey(credential), credential); err != nil {
			t.Fatal(err)
		}
	}
	second, exists, err := rowIDByAccountKey(ctx, store.db, "fixture", new("second"))
	if err != nil || !exists {
		t.Fatalf("second row: exists=%v err=%v", exists, err)
	}
	foreign, exists, err := rowIDByAccountKey(ctx, store.db, "other", new("foreign"))
	if err != nil || !exists {
		t.Fatalf("foreign row: exists=%v err=%v", exists, err)
	}
	for _, test := range []struct{ name, selection, want string }{
		{"selected later row", strconv.FormatInt(second, 10), "second"},
		{"Unicode whitespace", "\u00a0" + strconv.FormatInt(second, 10) + "\u00a0", "second"},
		{"foreign provider", strconv.FormatInt(foreign, 10), "first"},
		{"missing row", "999999", "first"},
		{"invalid selection", "not-an-id", "first"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.db.ExecContext(ctx, "UPDATE cache SET value = ? WHERE key = ?", test.selection, selectionKey("fixture")); err != nil {
				t.Fatal(err)
			}
			// Repeated reads of the later selected row exercise the single
			// connection pool: the joined row must close before the ID lookup.
			for range 4 {
				got, err := store.Read(ctx, "fixture")
				if err != nil || got == nil || got.AccountID != test.want {
					t.Fatalf("Read = %+v, %v; want %q", got, err, test.want)
				}
			}
		})
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE auth_credentials SET disabled_cause = 'logout' WHERE id = ?", second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE cache SET value = ? WHERE key = ?", strconv.FormatInt(second, 10), selectionKey("fixture")); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(ctx, "fixture")
	if err != nil || got == nil || got.AccountID != "first" {
		t.Fatalf("disabled selection: %+v, %v", got, err)
	}
}
