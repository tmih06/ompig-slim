package ai_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestRouterAdoptsExternalQuotaReset covers deletion and shorter replacement of
// persisted blocks without ClearResetBlocks on the router that recorded them.
// Only a real successful attempt confirms the recovered account again.
func TestRouterAdoptsExternalQuotaReset(t *testing.T) {
	for _, provider := range []string{"openai-codex", "anthropic"} {
		for _, replacement := range []bool{false, true} {
			t.Run(provider+map[bool]string{false: "/deleted", true: "/replaced"}[replacement], func(t *testing.T) {
				ctx := context.Background()
				key := keyOf("a")
				store := newFakeAccountStore(accountInfo(provider, key, true), accountInfo(provider, keyOf("b"), false))
				store.put(provider, key, ai.Credential{Type: ai.CredentialOAuth, Access: codexToken("plus")})
				store.put(provider, keyOf("b"), ai.Credential{Type: ai.CredentialOAuth, Access: codexToken("plus")})
				router := ai.NewAccountRouter(store)
				model := okModel(provider, "gpt-5")
				block := router.Fail(ctx, model, &store.accounts[0], "insufficient_quota", &ai.ProviderResponse{Status: 429, Headers: map[string]string{"retry-after": "3600"}})
				if block == nil {
					t.Fatal("quota rejection was not blocked")
				}
				// A sibling process spends a reset. A later failure may replace the
				// removed row with a shorter deadline; the old hour must not win.
				store.block(provider, key, block.Scope, 0)
				if replacement {
					short := time.Now().Add(time.Minute).UnixMilli()
					store.block(provider, key, block.Scope, short)
					until, err := router.BlockedUntil(ctx, model, &store.accounts[0])
					if err != nil || until == nil || *until != short {
						t.Fatalf("replacement deadline = %v, err = %v; want %d", until, err, short)
					}
					store.block(provider, key, block.Scope, 0)
				}
				var tried []string
				result := router.StreamRoutes(ctx, model, ai.AccountRouteRequest{SessionID: "reset"}, func(_ context.Context, selection ai.AccountSelection, _ ai.AccountObserve, _ bool) (*ai.AssistantMessageEventStream, error) {
					tried = append(tried, *selection.AccountKey)
					return okStream(), nil
				})
				for range result.Events(ctx) {
				}
				if len(tried) != 1 || tried[0] != "a" {
					t.Fatalf("reset account was skipped: %v", tried)
				}
			})
		}
	}
}

// TestRouterObservesSiblingSQLiteReset exercises the shared agent.db contract
// with two independent handles; no in-process reset notification is delivered.
func TestRouterObservesSiblingSQLiteReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.db")
	store, err := ai.OpenSQLiteAuthStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sibling, err := ai.OpenSQLiteAuthStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	defer sibling.Close()
	ctx := context.Background()
	key := keyOf("a")
	if _, err := store.SaveAccount(ctx, "anthropic", key, ai.Credential{Type: ai.CredentialOAuth, Access: "token", Refresh: "refresh", Expires: time.Now().Add(time.Hour).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	router := ai.NewAccountRouter(store)
	model := okModel("anthropic", "claude")
	account := accountInfo("anthropic", key, true)
	if router.Fail(ctx, model, &account, "insufficient_quota", nil) == nil {
		t.Fatal("quota rejection was not blocked")
	}
	if err := sibling.ClearAccountBlocks(ctx, "anthropic", key, []string{""}); err != nil {
		t.Fatal(err)
	}
	until, err := router.BlockedUntil(ctx, model, &account)
	if err != nil || until != nil {
		t.Fatalf("sibling reset did not retire the local block: %v, %v", until, err)
	}
}

// TestRouterExternalResetPreservesOtherBlocks ensures quota healing cannot
// remove independent auth blocks or local protection after a failed write.
func TestRouterExternalResetPreservesOtherBlocks(t *testing.T) {
	ctx := context.Background()
	key := keyOf("a")
	for _, failedWrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "auth", true: "failed-write"}[failedWrite], func(t *testing.T) {
			store := newFakeAccountStore(accountInfo("openai-codex", key, true))
			router := ai.NewAccountRouter(store)
			model := okModel("openai-codex", "gpt-5")
			if failedWrite {
				store.blockErr = errors.New("write failed")
			}
			quota := router.Fail(ctx, model, &store.accounts[0], "insufficient_quota", nil)
			want := quota.Until
			if !failedWrite {
				auth := router.Fail(ctx, model, &store.accounts[0], "invalid access token", nil)
				want = auth.Until
			}
			store.block("openai-codex", key, "chat", 0)
			until, err := router.BlockedUntil(ctx, model, &store.accounts[0])
			if err != nil || until == nil || *until != want {
				t.Fatalf("independent protection lost: deadline = %v, err = %v", until, err)
			}
		})
	}
}
