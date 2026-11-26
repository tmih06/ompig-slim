package broker

import (
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestRemoteLatencyAccountMetadataSharesOneBoundedView coalesces simultaneous
// non-secret account reads, reuses that view inside startup's one-second
// credential-snapshot horizon, expires it, and invalidates it for local mutations.
func TestRemoteLatencyAccountMetadataSharesOneBoundedView(t *testing.T) {
	client, host, traffic := delayedBroker(t, 50*time.Millisecond)
	ctx := t.Context()
	if _, err := host.vault.SaveAccount(ctx, "fixture", new("fixture"), ai.Credential{Type: ai.CredentialAPIKey, Key: "fixture-key"}); err != nil {
		t.Fatal(err)
	}
	store := &remoteStore{client: client}
	const callers = 8
	start := make(chan struct{})
	type result struct {
		accounts []ai.CredentialAccountInfo
		err      error
	}
	results := make(chan result, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			accounts, err := store.ListAccounts(ctx)
			results <- result{accounts: accounts, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var first, second []ai.CredentialAccountInfo
	for outcome := range results {
		if outcome.err != nil || len(outcome.accounts) != 1 {
			t.Fatalf("concurrent metadata read: %+v %v", outcome.accounts, outcome.err)
		}
		if first == nil {
			first = outcome.accounts
		} else if second == nil {
			second = outcome.accounts
		}
	}
	if got := traffic.requests.Load(); got != 1 {
		t.Fatalf("concurrent metadata reads used %d broker round trips, want 1", got)
	}
	*first[0].AccountKey = "mutated"
	if second[0].AccountKey == nil || *second[0].AccountKey != "fixture" {
		t.Fatal("shared metadata callers received the same mutable account key")
	}
	if _, err := store.ListAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	if got := traffic.requests.Load(); got != 1 {
		t.Fatalf("startup metadata phases did not share the bounded view: %d requests", got)
	}

	key := "later"
	if _, err := host.vault.SaveAccount(ctx, "fixture", &key, ai.Credential{Type: ai.CredentialAPIKey, Key: "fixture-key"}); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.ListAccounts(ctx)
	if err != nil || len(accounts) != 1 {
		t.Fatalf("bounded metadata view should initially retain sibling state: %+v %v", accounts, err)
	}
	client.accountsMu.Lock()
	client.accountsAt = time.Now().Add(-2 * snapshotCacheTTL)
	client.accountsMu.Unlock()
	accounts, err = store.ListAccounts(ctx)
	if err != nil || len(accounts) != 2 {
		t.Fatalf("expired metadata view did not observe live host state: %+v %v", accounts, err)
	}
	if got := traffic.requests.Load(); got != 2 {
		t.Fatalf("expired metadata view used %d broker round trips, want 2", got)
	}

	before := traffic.requests.Load()
	if _, err := store.SaveAccount(ctx, "fixture", new("local"), ai.Credential{Type: ai.CredentialAPIKey, Key: "fixture-key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	if got := traffic.requests.Load() - before; got != 2 {
		t.Fatalf("local mutation did not invalidate metadata: mutation+read used %d requests, want 2", got)
	}
}

// TestRemoteLatencyStartupWarmsReadViewsTogether keeps independent broker views
// off startup's serial critical path: metadata and credentials overlap rather
// than costing one RTT each before document reconciliation completes.
func TestRemoteLatencyStartupWarmsReadViewsTogether(t *testing.T) {
	client, _, traffic := delayedBroker(t, 50*time.Millisecond)
	start := time.Now()
	client.warmReadViews(t.Context())
	t.Logf("warm read views: %s, requests=%d, peak=%d", time.Since(start), traffic.requests.Load(), traffic.peak.Load())
	if got := traffic.requests.Load(); got != 2 {
		t.Fatalf("warm read views used %d requests, want 2", got)
	}
	if traffic.peak.Load() < 2 {
		t.Fatal("credential and metadata warming paid serial remote round trips")
	}
	client.accountsMu.Lock()
	metadata := client.accountsLoad
	client.accountsMu.Unlock()
	client.snapshotMu.Lock()
	credentials := client.snapshotLoad
	client.snapshotMu.Unlock()
	if !metadata || !credentials {
		t.Fatalf("warm views incomplete: metadata=%v credentials=%v", metadata, credentials)
	}
}
