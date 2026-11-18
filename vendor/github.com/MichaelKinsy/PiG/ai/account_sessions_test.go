package ai_test

import (
	"context"
	"errors"
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

func sessionUsageFact(key, plan string, remaining float64, resetIn time.Duration, exhausted bool) ai.AccountSessionUsage {
	keyCopy := key
	remainingCopy := remaining
	resetAt := time.Now().Add(resetIn).UnixMilli()
	duration := int64(5 * time.Hour / time.Millisecond)
	return ai.AccountSessionUsage{
		AccountKey:    &keyCopy,
		AccountKeySet: true,
		FetchedAt:     time.Now().UnixMilli(),
		Plan:          plan,
		Windows: []ai.AccountSessionUsageWindow{{
			ID:                "primary",
			RemainingFraction: &remainingCopy,
			ResetsAt:          &resetAt,
			DurationMs:        &duration,
			Exhausted:         exhausted,
		}},
	}
}

func installSessionUsage(router *ai.AccountRouter, facts *[]ai.AccountSessionUsage) {
	router.SetSessionUsageSource(func(context.Context, string, string) ([]ai.AccountSessionUsage, error) {
		return *facts, nil
	})
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

func TestCodexConcurrentUsageSessionAssignments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.db")
	first := sessionStore(t, path)
	saveSessionAccounts(t, first, nil, "a", "b")
	stores := []*ai.SQLiteAuthStorage{first, sessionStore(t, path), sessionStore(t, path), sessionStore(t, path)}
	facts := []ai.AccountSessionUsage{
		sessionUsageFact("a", "free", 1, 5*time.Hour, false),
		sessionUsageFact("b", "free", 0.4, 3*time.Hour, false),
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[string]int{}
	for i := range 20 {
		wg.Go(func() {
			route := ai.AccountSessionRoute{SessionID: fmt.Sprintf("usage-%d", i), ModelID: "gpt-6-luna", Usage: facts}
			selection, err := stores[i%len(stores)].ResolveSessionAccount(t.Context(), "codex-json", route, true)
			if err != nil || selection == nil || selection.AccountKey == nil {
				t.Errorf("usage assignment failed: %v", err)
				return
			}
			mu.Lock()
			counts[*selection.AccountKey]++
			mu.Unlock()
		})
	}
	wg.Wait()
	if counts["a"] != 20 || counts["b"] != 0 {
		t.Fatalf("concurrent sessions did not choose the most remaining quota: %v", counts)
	}
	var same []string
	for i := range 12 {
		wg.Go(func() {
			route := ai.AccountSessionRoute{SessionID: "usage-shared", ModelID: "gpt-6-luna", Usage: facts}
			selection, err := stores[i%len(stores)].ResolveSessionAccount(t.Context(), "codex-json", route, true)
			if err != nil || selection == nil || selection.AccountKey == nil {
				t.Errorf("shared usage assignment failed: %v", err)
				return
			}
			mu.Lock()
			same = append(same, *selection.AccountKey)
			mu.Unlock()
		})
	}
	wg.Wait()
	for _, key := range same {
		if key != "a" {
			t.Fatalf("same-session usage callers disagreed: %v", same)
		}
	}
	selection, err := stores[0].ResolveSessionAccount(t.Context(), "codex-json", ai.AccountSessionRoute{SessionID: "usage-next", ModelID: "gpt-6-luna", Usage: facts}, true)
	if err != nil || selection == nil || selection.AccountKey == nil || *selection.AccountKey != "a" {
		t.Fatalf("another session did not choose the most remaining quota: %+v %v", selection, err)
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

func TestCodexUsageFactsPreferMostRemainingAndKeepPins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.db")
	store := sessionStore(t, path)
	for _, account := range []struct{ key, token string }{
		{"b", "unreadable-jwt"}, {"a", "unreadable-jwt"}, {"plus", codexToken("plus")},
	} {
		if _, err := store.SaveAccount(t.Context(), "codex-json", &account.key, ai.Credential{Type: ai.CredentialOAuth, Access: account.token}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.SelectAccount(t.Context(), "codex-json", "plus"); err != nil {
		t.Fatal(err)
	}
	facts := []ai.AccountSessionUsage{
		sessionUsageFact("a", "free", 1, 5*time.Hour, false),
		sessionUsageFact("b", "free", 0.3, 3*time.Hour, false),
		sessionUsageFact("plus", "plus", 0.8, 4*time.Hour, false),
	}
	router := ai.NewAccountRouter(store)
	installSessionUsage(router, &facts)
	for _, test := range []struct{ session, model, want string }{
		{"s1", "gpt-6-luna", "a"},
		{"s2", "gpt-6-luna", "a"},
		{"s1", "gpt-6-luna", "a"},
		{"s1", "gpt-6.1-sol", "plus"},
		{"s3", "gpt-6-luna", "a"},
		{"s4", "gpt-6-luna", "a"},
	} {
		if got := sessionAttempt(t, router, test.session, test.model); got != test.want {
			t.Fatalf("session %s on %s used %s, want %s", test.session, test.model, got, test.want)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := sessionStore(t, path)
	facts = []ai.AccountSessionUsage{
		sessionUsageFact("a", "free", 0.4, 4*time.Hour, false),
		sessionUsageFact("b", "free", 1, 5*time.Hour, false),
		sessionUsageFact("plus", "plus", 0.8, 4*time.Hour, false),
	}
	router = ai.NewAccountRouter(reopened)
	installSessionUsage(router, &facts)
	if got := sessionAttempt(t, router, "s1", "gpt-6-luna"); got != "a" {
		t.Fatalf("fresh window facts moved a healthy pin after reopen: %s", got)
	}
	if got := sessionAttempt(t, router, "s5", "gpt-6-luna"); got != "b" {
		t.Fatalf("new session did not choose the newly higher quota after reopen: %s", got)
	}
}

func TestCodexUsagePlanOverridesJWTClaims(t *testing.T) {
	store := sessionStore(t, filepath.Join(t.TempDir(), "agent.db"))
	for _, account := range []struct{ key, plan string }{{"jwt-free", "free"}, {"conflict/plus", "plus"}} {
		if _, err := store.SaveAccount(t.Context(), "codex-json", &account.key, ai.Credential{Type: ai.CredentialOAuth, Access: codexToken(account.plan)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.SelectAccount(t.Context(), "codex-json", "conflict/plus"); err != nil {
		t.Fatal(err)
	}
	facts := []ai.AccountSessionUsage{
		sessionUsageFact("jwt-free", "plus", 0.7, 4*time.Hour, false),
		sessionUsageFact("conflict/plus", "free", 0.8, 5*time.Hour, false),
	}
	router := ai.NewAccountRouter(store)
	installSessionUsage(router, &facts)
	if got := sessionAttempt(t, router, "luna", "gpt-6-luna"); got != "conflict/plus" {
		t.Fatalf("fresh free usage did not override the plus JWT plan: %s", got)
	}
	if got := sessionAttempt(t, router, "paid", "gpt-6.1-sol"); got != "jwt-free" {
		t.Fatalf("fresh paid usage did not override the free JWT plan: %s", got)
	}
}

func TestCodexUsagePreviewsDoNotPinAndExhaustedPinsMove(t *testing.T) {
	store := sessionStore(t, filepath.Join(t.TempDir(), "agent.db"))
	saveSessionAccounts(t, store, nil, "a", "b")
	if _, err := store.SelectAccount(t.Context(), "codex-json", "b"); err != nil {
		t.Fatal(err)
	}
	facts := []ai.AccountSessionUsage{
		sessionUsageFact("a", "free", 1, 5*time.Hour, false),
		sessionUsageFact("b", "free", 0.4, 3*time.Hour, false),
	}
	router := ai.NewAccountRouter(store)
	installSessionUsage(router, &facts)
	for range 3 {
		key, present, err := router.ResolveAccount(t.Context(), okModel("codex-json", "gpt-6-luna"), ai.AccountRouteRequest{SessionID: "preview"})
		if err != nil || !present || key == nil || *key != "a" {
			t.Fatalf("preview selected %v present=%t err=%v, want account a with the most remaining quota", key, present, err)
		}
	}
	facts[0] = sessionUsageFact("a", "free", 0.2, time.Hour, false)
	if got := sessionAttempt(t, router, "preview", "gpt-6-luna"); got != "b" {
		t.Fatalf("preview pinned an account: %s", got)
	}
	facts[1] = sessionUsageFact("b", "free", 0, time.Hour, true)
	if got := sessionAttempt(t, router, "preview", "gpt-6-luna"); got != "a" {
		t.Fatalf("fresh exhaustion did not retire the pinned account: %s", got)
	}
	facts[1] = sessionUsageFact("b", "free", 1, time.Hour, false)
	if got := sessionAttempt(t, router, "preview", "gpt-6-luna"); got != "a" {
		t.Fatalf("recovered quota displaced the fallback pin: %s", got)
	}
}

func TestCodexUsageRemainingIgnoresResetTimeAndUsesShortestWindow(t *testing.T) {
	t.Run("reset time does not outweigh remaining quota", func(t *testing.T) {
		store := sessionStore(t, filepath.Join(t.TempDir(), "agent.db"))
		saveSessionAccounts(t, store, nil, "short", "steady")
		if _, err := store.SelectAccount(t.Context(), "codex-json", "steady"); err != nil {
			t.Fatal(err)
		}
		facts := []ai.AccountSessionUsage{
			sessionUsageFact("short", "free", 0.3, 15*time.Minute, false),
			sessionUsageFact("steady", "free", 0.5, 2*time.Hour, false),
		}
		selection, err := store.ResolveSessionAccount(t.Context(), "codex-json", ai.AccountSessionRoute{SessionID: "short", ModelID: "gpt-6-luna", Usage: facts}, true)
		if err != nil || selection == nil || selection.AccountKey == nil || *selection.AccountKey != "steady" {
			t.Fatalf("short reset outweighed remaining quota: %+v %v", selection, err)
		}
	})
	for _, tc := range []struct {
		name      string
		duration  *int64
		exhausted bool
		want      string
	}{
		{"longer window does not affect ranking", new((7 * 24 * time.Hour).Milliseconds()), false, "idle"},
		{"shorter secondary window determines rank", new(time.Hour.Milliseconds()), false, "steady"},
		{"missing window length prefers primary", nil, false, "idle"},
		{"exhausted longer window blocks account", new((7 * 24 * time.Hour).Milliseconds()), true, "steady"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := sessionStore(t, filepath.Join(t.TempDir(), "agent.db"))
			saveSessionAccounts(t, store, nil, "idle", "steady")
			idle := sessionUsageFact("idle", "free", 1, 5*time.Hour, false)
			idle.Windows = append(idle.Windows, ai.AccountSessionUsageWindow{
				ID: "secondary", RemainingFraction: new(0.3), DurationMs: tc.duration, Exhausted: tc.exhausted,
			})
			facts := []ai.AccountSessionUsage{idle, sessionUsageFact("steady", "free", 0.8, 2*time.Hour, false)}
			router := ai.NewAccountRouter(store)
			installSessionUsage(router, &facts)
			if got := sessionAttempt(t, router, "first", "gpt-6-luna"); got != tc.want {
				t.Fatalf("used %s, want %s", got, tc.want)
			}
		})
	}
	t.Run("hot primary", func(t *testing.T) {
		store := sessionStore(t, filepath.Join(t.TempDir(), "agent.db"))
		saveSessionAccounts(t, store, nil, "hot", "healthy")
		if _, err := store.SelectAccount(t.Context(), "codex-json", "hot"); err != nil {
			t.Fatal(err)
		}
		facts := []ai.AccountSessionUsage{
			sessionUsageFact("hot", "free", 0.08, 15*time.Minute, false),
			sessionUsageFact("healthy", "free", 0.05, 2*time.Hour, false),
		}
		selection, err := store.ResolveSessionAccount(t.Context(), "codex-json", ai.AccountSessionRoute{SessionID: "hot", ModelID: "gpt-6-luna", Usage: facts}, true)
		if err != nil || selection == nil || selection.AccountKey == nil || *selection.AccountKey != "hot" {
			t.Fatalf("larger remaining quota was deprioritized: %+v %v", selection, err)
		}
	})
}

func TestCodexUsageSourcePropagatesCancellationAndStoreErrors(t *testing.T) {
	store := sessionStore(t, filepath.Join(t.TempDir(), "agent.db"))
	saveSessionAccounts(t, store, nil, "a", "b")
	router := ai.NewAccountRouter(store)
	storeErr := errors.New("fixture account-store failure")
	router.SetSessionUsageSource(func(context.Context, string, string) ([]ai.AccountSessionUsage, error) {
		return nil, storeErr
	})
	if _, err := router.Candidates(t.Context(), okModel("codex-json", "gpt-6-luna"), ai.AccountRouteRequest{SessionID: "error"}); !errors.Is(err, storeErr) {
		t.Fatalf("usage source store error was swallowed: %v", err)
	}
	started := make(chan struct{})
	router.SetSessionUsageSource(func(ctx context.Context, _, _ string) ([]ai.AccountSessionUsage, error) {
		close(started)
		<-ctx.Done()
		return nil, context.Cause(ctx)
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := router.Candidates(ctx, okModel("codex-json", "gpt-6-luna"), ai.AccountRouteRequest{SessionID: "cancel"})
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("caller cancellation was swallowed: %v", err)
	}
}

func TestCodexGoPlanKeepsNoUsageRoutingPriority(t *testing.T) {
	store := sessionStore(t, filepath.Join(t.TempDir(), "agent.db"))
	saveSessionAccounts(t, store, map[string]string{"go": "go"}, "go", "plus")
	if _, err := store.SelectAccount(t.Context(), "codex-json", "plus"); err != nil {
		t.Fatal(err)
	}
	if got := sessionAttempt(t, ai.NewAccountRouter(store), "go-fallback", "gpt-6-luna"); got != "plus" {
		t.Fatalf("no-usage Go plan routing changed: %s", got)
	}
}

func TestCodexUsageIgnoresExpiredStaleAndUnknownFacts(t *testing.T) {
	for _, test := range []struct {
		name string
		fact ai.AccountSessionUsage
	}{
		{name: "expired window", fact: func() ai.AccountSessionUsage {
			fact := sessionUsageFact("a", "free", 0, -time.Hour, true)
			return fact
		}()},
		{name: "stale report", fact: func() ai.AccountSessionUsage {
			fact := sessionUsageFact("b", "free", 1, time.Hour, false)
			fact.FetchedAt = time.Now().Add(-ai.AccountSessionUsageFreshness - time.Second).UnixMilli()
			return fact
		}()},
		{name: "unknown plan", fact: ai.AccountSessionUsage{AccountKey: keyOf("b"), AccountKeySet: true, FetchedAt: time.Now().UnixMilli(), Plan: "future-tier"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := sessionStore(t, filepath.Join(t.TempDir(), "agent.db"))
			saveSessionAccounts(t, store, map[string]string{"a": "free"}, "a", "b")
			if _, err := store.SelectAccount(t.Context(), "codex-json", "a"); err != nil {
				t.Fatal(err)
			}
			facts := []ai.AccountSessionUsage{test.fact}
			router := ai.NewAccountRouter(store)
			installSessionUsage(router, &facts)
			if got := sessionAttempt(t, router, "fallback", "gpt-6-luna"); got != "a" {
				t.Fatalf("invalid usage changed credential-plan fallback: %s", got)
			}
		})
	}
}
