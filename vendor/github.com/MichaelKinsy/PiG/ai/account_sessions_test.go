package ai_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

func sessionStore(t *testing.T, path string) *ai.SQLiteAuthStorage {
	t.Helper()
	store, err := ai.OpenSQLiteAuthStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func saveSessionAccounts(t *testing.T, store *ai.SQLiteAuthStorage, plans map[string]string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		plan := plans[key]
		if plan == "" {
			plan = "plus"
		}
		_, err := store.SaveAccount(t.Context(), "codex-json", &key, ai.Credential{Type: ai.CredentialOAuth, Access: codexToken(plan)})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func sessionAttempt(t *testing.T, router *ai.AccountRouter, session, model string) string {
	t.Helper()
	var key string
	_, err := router.ImagesRoutes(t.Context(), okModel("codex-json", model), ai.AccountRouteRequest{SessionID: session}, func(_ context.Context, selection ai.AccountSelection, _ ai.AccountObserve, _ bool) (ai.AssistantImages, error) {
		if selection.AccountKey == nil {
			key = "<nil>"
		} else {
			key = *selection.AccountKey
		}
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestCodexSessionsDistributeAndSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.db")
	store := sessionStore(t, path)
	saveSessionAccounts(t, store, nil, "a", "b")
	router := ai.NewAccountRouter(store)
	for range 3 {
		key, present, err := router.ResolveAccount(t.Context(), okModel("codex-json", "gpt-6.1-sol"), ai.AccountRouteRequest{SessionID: "preview"})
		if err != nil || !present || key == nil || *key != "b" {
			t.Fatalf("preview did not retain initial selection: %v %v %v", key, present, err)
		}
	}
	for _, test := range []struct{ session, want string }{{"s1", "b"}, {"s2", "a"}, {"s3", "b"}, {"s1", "b"}} {
		if got := sessionAttempt(t, router, test.session, "gpt-6.1-sol"); got != test.want {
			t.Fatalf("session %s used %s, want %s", test.session, got, test.want)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := sessionStore(t, path)
	router = ai.NewAccountRouter(reopened)
	if got := sessionAttempt(t, router, "s2", "gpt-6.1-sol"); got != "a" {
		t.Fatalf("reopened session lost its assignment: %s", got)
	}
	if got := sessionAttempt(t, router, "s4", "gpt-6.1-sol"); got != "a" {
		t.Fatalf("reopening reset the cursor: %s", got)
	}
	if got := sessionAttempt(t, router, "s1", "another-model"); got != "b" {
		t.Fatalf("model-specific assignment did not use shared cursor: %s", got)
	}
	if got := sessionAttempt(t, ai.NewAccountRouter(reopened), "", "gpt-6.1-sol"); got != "b" {
		t.Fatalf("sessionless request stopped using selection: %s", got)
	}
	accounts, err := reopened.ListAccounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		if account.Active != (*account.AccountKey == "b") {
			t.Fatal("routing changed global selection")
		}
	}
	if _, err := reopened.SelectAccount(t.Context(), "codex-json", "a"); err != nil {
		t.Fatal(err)
	}
	if got := sessionAttempt(t, router, "s1", "gpt-6.1-sol"); got != "a" {
		t.Fatalf("explicit selection did not reset assignments: %s", got)
	}
}

func TestCodexConcurrentSessionAssignments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.db")
	first := sessionStore(t, path)
	saveSessionAccounts(t, first, nil, "a", "b")
	stores := []*ai.SQLiteAuthStorage{first, sessionStore(t, path), sessionStore(t, path), sessionStore(t, path)}
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[string]int{}
	for i := range 20 {
		wg.Go(func() {
			route := ai.AccountSessionRoute{SessionID: fmt.Sprintf("s%d", i), ModelID: "gpt-6.1-sol"}
			selection, err := stores[i%len(stores)].ResolveSessionAccount(t.Context(), "codex-json", route, true)
			if err != nil || selection == nil || selection.AccountKey == nil {
				t.Errorf("assignment failed: %v", err)
				return
			}
			mu.Lock()
			counts[*selection.AccountKey]++
			mu.Unlock()
		})
	}
	wg.Wait()
	if counts["a"] != 10 || counts["b"] != 10 {
		t.Fatalf("cross-connection distribution is not balanced: %v", counts)
	}
	var same []string
	for i := range 12 {
		wg.Go(func() {
			selection, err := stores[i%len(stores)].ResolveSessionAccount(t.Context(), "codex-json", ai.AccountSessionRoute{SessionID: "shared", ModelID: "gpt-6.1-sol"}, true)
			if err != nil || selection == nil || selection.AccountKey == nil {
				t.Errorf("shared assignment failed: %v", err)
				return
			}
			mu.Lock()
			same = append(same, *selection.AccountKey)
			mu.Unlock()
		})
	}
	wg.Wait()
	for _, key := range same {
		if key != "b" {
			t.Fatalf("same-session concurrent callers disagreed: %v", same)
		}
	}
	if got := sessionAttempt(t, ai.NewAccountRouter(first), "after-shared", "gpt-6.1-sol"); got != "a" {
		t.Fatalf("duplicate reservations spent round-robin turns: %s", got)
	}
}

func TestCodexSessionPriorityAndBlocks(t *testing.T) {
	store := sessionStore(t, filepath.Join(t.TempDir(), "agent.db"))
	saveSessionAccounts(t, store, map[string]string{"free": "free"}, "free", "paid")
	router := ai.NewAccountRouter(store)
	if got := sessionAttempt(t, router, "one", "gpt-6-luna"); got != "free" {
		t.Fatalf("free priority was lost: %s", got)
	}
	if got := sessionAttempt(t, router, "two", "gpt-6.1-sol"); got != "paid" {
		t.Fatalf("ineligible free account selected: %s", got)
	}
	if err := store.BlockAccount(t.Context(), "codex-json", keyOf("free"), "chat", time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if got := sessionAttempt(t, router, "one", "gpt-6-luna"); got != "paid" {
		t.Fatalf("blocked free account selected: %s", got)
	}
	if err := store.ClearAccountBlocks(t.Context(), "codex-json", keyOf("free"), []string{"chat"}); err != nil {
		t.Fatal(err)
	}
	if got := sessionAttempt(t, router, "one", "gpt-6-luna"); got != "free" {
		t.Fatalf("paid pin overrode available free account: %s", got)
	}
}

func TestCodexSessionFallbackSticksAfterBlockCleared(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.db")
	store := sessionStore(t, path)
	saveSessionAccounts(t, store, nil, "a", "b")
	router := ai.NewAccountRouter(store)
	var tried []string
	_, err := router.ImagesRoutes(t.Context(), okModel("codex-json", "gpt-6.1-sol"), ai.AccountRouteRequest{SessionID: "fallback"}, func(_ context.Context, selection ai.AccountSelection, observe ai.AccountObserve, _ bool) (ai.AssistantImages, error) {
		key := *selection.AccountKey
		tried = append(tried, key)
		if key == "b" {
			observe(ai.ProviderResponse{Status: 429})
			return ai.AssistantImages{StopReason: ai.ImagesStopReasonError, ErrorMessage: "usage_limit_reached"}, nil
		}
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
	})
	if err != nil || fmt.Sprint(tried) != "[b a]" {
		t.Fatalf("fallback failed: %v %v", tried, err)
	}
	if err := store.ClearAccountBlocks(t.Context(), "codex-json", keyOf("b"), nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if got := sessionAttempt(t, ai.NewAccountRouter(sessionStore(t, path)), "fallback", "gpt-6.1-sol"); got != "a" {
		t.Fatalf("fallback assignment did not survive restart and quota reset: %s", got)
	}
}

func TestCodexIdentitylessSessionAssignment(t *testing.T) {
	store := sessionStore(t, filepath.Join(t.TempDir(), "agent.db"))
	if _, err := store.SaveAccount(t.Context(), "codex-json", nil, ai.Credential{Type: ai.CredentialOAuth, Access: codexToken("plus")}); err != nil {
		t.Fatal(err)
	}
	saveSessionAccounts(t, store, nil, "named")
	router := ai.NewAccountRouter(store)
	if got := sessionAttempt(t, router, "one", "gpt-6.1-sol"); got != "named" {
		t.Fatal(got)
	}
	if got := sessionAttempt(t, router, "two", "gpt-6.1-sol"); got != "<nil>" {
		t.Fatalf("nil identity confused with provider selection: %s", got)
	}
	if got := sessionAttempt(t, ai.NewAccountRouter(store), "two", "gpt-6.1-sol"); got != "<nil>" {
		t.Fatalf("nil identity pin was lost: %s", got)
	}
}
