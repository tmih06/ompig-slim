package broker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding"
)

// latencyTraffic counts real HTTP exchanges, not simulated delays or cache hits.
type latencyTraffic struct {
	requests  atomic.Int64
	snapshots atomic.Int64
	active    atomic.Int64
	peak      atomic.Int64
}

// delayedBroker adds one cancellable RTT per HTTP exchange with a real vault.
// Concurrent exchanges overlap, as they do across a remote network; no provider
// traffic or real credentials are used. Setup bypasses the delay and counters.
func delayedBroker(t testing.TB, rtt time.Duration) (*Client, *Store, *latencyTraffic) {
	t.Helper()
	client, _, host := testClients(t)
	store := host.Config.Handler.(*Store)
	traffic := &latencyTraffic{}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traffic.requests.Add(1)
		if r.URL.Path == "/v1/snapshot" {
			traffic.snapshots.Add(1)
		}
		active := traffic.active.Add(1)
		defer traffic.active.Add(-1)
		for peak := traffic.peak.Load(); active > peak; peak = traffic.peak.Load() {
			if traffic.peak.CompareAndSwap(peak, active) {
				break
			}
		}
		timer := time.NewTimer(rtt)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
		store.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	client.base = proxy.URL
	client.http = proxy.Client()
	return client, store, traffic
}

// TestRemoteLatencyIndependentPreferencesOverlap requires startup/idle/exit
// reconciliation to overlap independent documents instead of paying five RTTs.
// The structural assertion is stable even on heavily loaded CI hosts.
func TestRemoteLatencyIndependentPreferencesOverlap(t *testing.T) {
	client, _, traffic := delayedBroker(t, 50*time.Millisecond)
	start := time.Now()
	if err := client.SyncFiles(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Logf("preference startup: %s, requests=%d, peak=%d", time.Since(start), traffic.requests.Load(), traffic.peak.Load())
	if traffic.peak.Load() < 2 {
		t.Fatal("independent preferences paid serial remote round trips")
	}
}

// TestRemoteLatencyQuotaReadKeepsCredentialView pins the auth -> quota -> auth
// chain used during account routing. Reading live quota must not discard an
// unchanged credential view and force another full token-bearing snapshot.
func TestRemoteLatencyQuotaReadKeepsCredentialView(t *testing.T) {
	client, host, traffic := delayedBroker(t, 50*time.Millisecond)
	if _, err := host.vault.SaveAccount(t.Context(), "fixture", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "fixture-key"}); err != nil {
		t.Fatal(err)
	}
	store := &remoteStore{client: client}
	start := time.Now()
	for range 2 {
		credential, err := store.Read(t.Context(), "fixture")
		if err != nil || credential == nil || credential.Key != "fixture-key" {
			t.Fatalf("credential: %v, %v", credential, err)
		}
		if _, err := store.GetAccountBlockedUntil(t.Context(), "fixture", nil, []string{"chat"}); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("auth/quota routing: %s, requests=%d, snapshots=%d", time.Since(start), traffic.requests.Load(), traffic.snapshots.Load())
	if got := traffic.snapshots.Load(); got != 1 {
		t.Fatalf("quota reads forced %d credential snapshots, want 1", got)
	}
}

// TestRemoteLatencyQuotaStaysLive proves that retaining access credentials does
// not cache quota: a sibling's block and clear must affect the very next read.
func TestRemoteLatencyQuotaStaysLive(t *testing.T) {
	client, host, traffic := delayedBroker(t, 20*time.Millisecond)
	store := &remoteStore{client: client}
	ctx := t.Context()
	if _, err := host.vault.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "fixture-key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Hour).UnixMilli()
	if err := host.vault.BlockAccount(ctx, "fixture", nil, "chat", until); err != nil {
		t.Fatal(err)
	}
	blocked, err := store.GetAccountBlockedUntil(ctx, "fixture", nil, []string{"chat"})
	if err != nil || blocked == nil || *blocked != until {
		t.Fatalf("sibling block missed: %v, %v", blocked, err)
	}
	if err := host.vault.ClearAccountBlocks(ctx, "fixture", nil, nil); err != nil {
		t.Fatal(err)
	}
	if blocked, err := store.GetAccountBlockedUntil(ctx, "fixture", nil, []string{"chat"}); err != nil || blocked != nil {
		t.Fatalf("sibling clear missed: %v, %v", blocked, err)
	}
	if _, err := store.Read(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	if traffic.snapshots.Load() != 1 {
		t.Fatal("live quota reads discarded unchanged credentials")
	}
}

// TestRemoteLatencyMutationsAndExpiryStillInvalidate checks selection, logout,
// cross-client changes after TTL, and unavailable-host refusal with a warm view.
func TestRemoteLatencyMutationsAndExpiryStillInvalidate(t *testing.T) {
	client, host, _ := delayedBroker(t, 20*time.Millisecond)
	store := &remoteStore{client: client}
	ctx := t.Context()
	for _, key := range []string{"one", "two"} {
		if _, err := store.SaveAccount(ctx, "fixture", &key, ai.Credential{Type: ai.CredentialAPIKey, Key: key}); err != nil {
			t.Fatal(err)
		}
	}
	assertKey := func(want string) {
		t.Helper()
		credential, err := store.Read(ctx, "fixture")
		if err != nil || credential == nil || credential.Key != want {
			t.Fatalf("active credential must be %s: %v, %v", want, credential, err)
		}
	}
	assertKey("two")
	if selected, err := store.SelectAccount(ctx, "fixture", "one"); err != nil || !selected {
		t.Fatalf("select: %v, %v", selected, err)
	}
	assertKey("one")
	if deleted, err := store.DeleteAccount(ctx, "fixture", "one"); err != nil || !deleted {
		t.Fatalf("delete: %v, %v", deleted, err)
	}
	assertKey("two")
	if err := host.vault.Delete(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	// Advance the view age without a one-second sleep; no requests are in flight.
	client.snapshotAt = time.Now().Add(-2 * snapshotCacheTTL)
	if credential, err := store.Read(ctx, "fixture"); err != nil || credential != nil {
		t.Fatalf("expired view hid sibling logout: %v, %v", credential, err)
	}
	client.snapshotAt = time.Now().Add(-2 * snapshotCacheTTL)
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(ctx, "fixture"); err == nil {
		t.Fatal("expired view hid unavailable broker")
	}
}

// TestRemoteLatencyPreferencesPreserveIndependentFailures exercises delayed
// adoption, backup, conflicting writes, deletion and retry through real CAS.
func TestRemoteLatencyPreferencesPreserveIndependentFailures(t *testing.T) {
	client, host, _ := delayedBroker(t, 20*time.Millisecond)
	ctx := t.Context()
	mustWrite(t, filepath.Join(host.dir, "settings.json"), `{"host":true}`)
	mustWrite(t, filepath.Join(client.dir, "settings.json"), `{"local":true}`)
	if err := client.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	backup, err := readLocal(client.statePath("settings.json.backup"))
	if err != nil || string(backup) != `{"local":true}` {
		t.Fatalf("adoption lost backup: %s, %v", backup, err)
	}
	mustWrite(t, filepath.Join(host.dir, "settings.json"), `{"hostEdit":true}`)
	mustWrite(t, filepath.Join(client.dir, "settings.json"), `{"localEdit":true}`)
	mustWrite(t, filepath.Join(host.dir, "keybindings.json"), `{"app.interrupt":"ctrl+x"}`)
	if err := client.SyncFiles(ctx); err == nil {
		t.Fatal("simultaneous edits must conflict")
	}
	settings, err := readLocal(filepath.Join(client.dir, "settings.json"))
	if err != nil || string(settings) != `{"localEdit":true}` {
		t.Fatalf("conflict lost local edit: %s, %v", settings, err)
	}
	keybindings, err := readLocal(filepath.Join(client.dir, "keybindings.json"))
	if err != nil || string(keybindings) != `{"app.interrupt":"ctrl+x"}` {
		t.Fatalf("conflict stopped independent file: %s, %v", keybindings, err)
	}
	mustWrite(t, filepath.Join(client.dir, "settings.json"), `{"hostEdit":true}`)
	if err := client.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(client.dir, "keybindings.json")); err != nil {
		t.Fatal(err)
	}
	if err := client.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if data, err := readLocal(filepath.Join(host.dir, "keybindings.json")); err != nil || data != nil {
		t.Fatalf("deletion lost: %s, %v", data, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := client.SyncFiles(canceled); err == nil {
		t.Fatal("canceled sync must report failure")
	}
	if err := client.SyncFiles(ctx); err != nil {
		t.Fatal("cancellation retained profile lock:", err)
	}
}

// BenchmarkRemoteLatency measures isolated lifecycle paths at loopback, typical
// WAN and high RTT. Requests/op attributes latency to network work; every sample
// starts with a cold credential view so TTL hits cannot hide a startup cost.
// Services readiness includes preference sync, registry construction, model
// availability and auth resolution, but not a terminal or model inference.
func BenchmarkRemoteLatency(b *testing.B) {
	for _, rtt := range []time.Duration{0, 50 * time.Millisecond, 150 * time.Millisecond} {
		b.Run(fmt.Sprintf("RTT=%s", rtt), func(b *testing.B) {
			for _, path := range []string{"startup", "idle-sync", "push", "pull", "ready", "auth-quota", "select", "logout"} {
				b.Run(path, func(b *testing.B) {
					client, host, traffic := delayedBroker(b, rtt)
					ctx := context.Background()
					store := &remoteStore{client: client}
					if _, err := host.vault.SaveAccount(ctx, "openai", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "fixture-key"}); err != nil {
						b.Fatal(err)
					}
					if path != "startup" && path != "ready" {
						if err := client.SyncFiles(ctx); err != nil {
							b.Fatal(err)
						}
					}
					installRemoteStore(client)
					b.ResetTimer()
					var requests int64
					for i := 0; i < b.N; i++ {
						b.StopTimer()
						client.invalidateSnapshot()
						if path == "startup" || path == "ready" {
							client.dir = b.TempDir()
						}
						if path == "push" {
							if err := replaceFile(filepath.Join(client.dir, "settings.json"), []byte(fmt.Sprintf(`{"sample":%d}`, i))); err != nil {
								b.Fatal(err)
							}
						}
						if path == "pull" {
							if err := replaceFile(filepath.Join(host.dir, "settings.json"), []byte(fmt.Sprintf(`{"sample":%d}`, i))); err != nil {
								b.Fatal(err)
							}
						}
						if path == "select" || path == "logout" {
							if _, err := host.vault.SaveAccount(ctx, "openai", new("named"), ai.Credential{Type: ai.CredentialAPIKey, Key: "fixture-key"}); err != nil {
								b.Fatal(err)
							}
						}
						before := traffic.requests.Load()
						b.StartTimer()
						var err error
						switch path {
						case "startup", "idle-sync", "push", "pull":
							err = client.SyncFiles(ctx)
						case "ready":
							err = client.SyncFiles(ctx)
							if err == nil {
								var services *coding.AgentSessionServices
								services, err = coding.CreateAgentSessionServices(coding.CreateAgentSessionServicesOptions{AgentDir: client.dir, CWD: client.dir, ProjectTrusted: new(false)})
								if err == nil {
									_, err = services.Registry().GetAvailableAllModelDataContext(ctx, "openai")
									if err == nil {
										_, err = services.Registry().ResolveRegistryProviderAuth(ctx, "openai")
									}
									services.Close()
								}
							}
						case "auth-quota":
							for range 2 {
								if _, err = store.Read(ctx, "openai"); err != nil {
									break
								}
								if _, err = store.GetAccountBlockedUntil(ctx, "openai", nil, []string{"chat"}); err != nil {
									break
								}
							}
						case "select":
							_, err = store.SelectAccount(ctx, "openai", "named")
						case "logout":
							_, err = store.DeleteAccount(ctx, "openai", "named")
						}
						if err != nil {
							b.Fatal(err)
						}
						requests += traffic.requests.Load() - before
					}
					b.ReportMetric(float64(requests)/float64(b.N), "requests/op")
				})
			}
		})
	}
}
