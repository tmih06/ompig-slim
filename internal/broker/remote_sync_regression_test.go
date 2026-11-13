package broker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding"
)

// brokerOAuthTransport models Google's rejection of a redacted refresh token.
// It never sends requests or fixture credentials to a real provider.
type brokerOAuthTransport struct{ calls atomic.Int32 }

// RoundTrip accepts only the host's private refresh and returns a fresh grant.
func (transport *brokerOAuthTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.String() != "https://oauth2.googleapis.com/token" {
		return nil, fmt.Errorf("unexpected OAuth endpoint")
	}
	if err := request.ParseForm(); err != nil {
		return nil, err
	}
	transport.calls.Add(1)
	status, body := 200, `{"access_token":"fresh-access","expires_in":3600}`
	if request.Form.Get("refresh_token") != "host-only-refresh" {
		status, body = 400, `{"error":"invalid_grant","error_description":"Bad Request"}`
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

// TestRegistryRefreshUsesBroker pins the real session registry boundary, which
// sits between inference auth and the runtime-key overlay around the remote store.
func TestRegistryRefreshUsesBroker(t *testing.T) {
	for _, scenario := range []struct {
		name, provider    string
		key               *string
		expires           int64
		force, runtimeKey bool
	}{
		{name: "expired-antigravity", provider: ai.GoogleAntigravityProviderID, expires: 1},
		{name: "expired-gemini", provider: ai.GoogleGeminiCliProviderID, expires: 1},
		{name: "named-account", provider: ai.GoogleAntigravityProviderID, key: new("requested"), expires: 1},
		{name: "near-expiry", provider: ai.GoogleAntigravityProviderID, expires: time.Now().Add(time.Minute).UnixMilli()},
		{name: "forced-recovery", provider: ai.GoogleAntigravityProviderID, expires: time.Now().Add(time.Hour).UnixMilli(), force: true},
		{name: "valid-access", provider: ai.GoogleAntigravityProviderID, expires: time.Now().Add(time.Hour).UnixMilli()},
		{name: "runtime-key", provider: ai.GoogleAntigravityProviderID, expires: 1, runtimeKey: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			client, _, server := testClients(t)
			store := &remoteStore{client: client}
			transport := &brokerOAuthTransport{}
			previous := http.DefaultClient
			http.DefaultClient = &http.Client{Transport: transport}
			t.Cleanup(func() { http.DefaultClient = previous })
			ctx := context.Background()
			if _, err := store.SaveAccount(ctx, scenario.provider, scenario.key, ai.Credential{Type: ai.CredentialOAuth, Access: "original-access", Refresh: "host-only-refresh", Expires: scenario.expires, ProjectID: "fixture-project"}); err != nil {
				t.Fatal(err)
			}
			// Leave a different account selected: exact-account refresh must not
			// borrow its grant or change the broker's shared selection.
			if scenario.key != nil {
				if _, err := store.SaveAccount(ctx, scenario.provider, new("selected"), ai.Credential{Type: ai.CredentialOAuth, Access: "other-access", Refresh: "other-refresh", Expires: 1, ProjectID: "other-project"}); err != nil {
					t.Fatal(err)
				}
			}
			installRemoteStore(client)
			services, err := coding.CreateAgentSessionServices(coding.CreateAgentSessionServicesOptions{AgentDir: client.dir, CWD: client.dir, ProjectTrusted: new(false)})
			if err != nil {
				t.Fatal(err)
			}
			defer services.Close()
			overrides := ai.AuthResolutionOverrides{ForceOAuthRefresh: scenario.force}
			if scenario.key != nil {
				overrides = ai.WithAccountKey(overrides, scenario.key)
			}
			want, wantCalls := "fresh-access", int32(1)
			if !scenario.force && scenario.expires > time.Now().Add(10*time.Minute).UnixMilli() {
				want, wantCalls = "original-access", 0
			}
			if scenario.runtimeKey {
				services.Registry().SetRuntimeAPIKey(scenario.provider, "explicit-runtime-key")
				want, wantCalls = "explicit-runtime-key", 0
			}
			resolved, err := services.Registry().ResolveRegistryProviderAuth(ctx, scenario.provider, overrides)
			if err != nil || resolved == nil || !strings.Contains(resolved.Auth.APIKey, want) {
				t.Fatalf("session must refresh through broker, not send redaction to Google: %v, %v", resolved, err)
			}
			if transport.calls.Load() != wantCalls {
				t.Fatalf("refresh exchanges = %d, want %d", transport.calls.Load(), wantCalls)
			}
			host := server.Config.Handler.(*Store)
			raw, err := host.vault.ReadAccount(ctx, scenario.provider, scenario.key)
			if err != nil || raw == nil || raw.Refresh != "host-only-refresh" || raw.ProjectID != "fixture-project" {
				t.Fatal("refresh lost host credential or project binding", err)
			}
			if scenario.key != nil {
				selected, err := host.vault.Read(ctx, scenario.provider)
				if err != nil || selected == nil || selected.Access != "other-access" {
					t.Fatal("exact refresh changed selected account", err)
				}
			}
		})
	}
}

// TestPreferenceConflictDoesNotStopOtherFiles pins independent document sync:
// conflicting settings must not prevent keybindings or models from propagating.
func TestPreferenceConflictDoesNotStopOtherFiles(t *testing.T) {
	a, b, _ := testClients(t)
	ctx := context.Background()
	mustWrite(t, filepath.Join(a.dir, "settings.json"), `{"base":true}`)
	if err := a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(a.dir, "settings.json"), `{"a":true}`)
	mustWrite(t, filepath.Join(b.dir, "settings.json"), `{"b":true}`)
	mustWrite(t, filepath.Join(a.dir, "keybindings.json"), `{"app.interrupt":"ctrl+x"}`)
	if err := a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.SyncFiles(ctx); err == nil {
		t.Fatal("conflict was hidden")
	}
	data, err := readLocal(filepath.Join(b.dir, "keybindings.json"))
	if err != nil || string(data) != `{"app.interrupt":"ctrl+x"}` {
		t.Fatalf("independent keybindings did not sync: %s, %v", data, err)
	}
}

// TestStartRecoversAfterBrokerOutage exercises the owned five-second worker,
// not just a manual SyncFiles call or shutdown flush. Reconnection must not open
// a local credential mirror while the host is unavailable.
func TestStartRecoversAfterBrokerOutage(t *testing.T) {
	a, b, host := testClients(t)
	var available atomic.Bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !available.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		host.Config.Handler.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	t.Setenv(EnvURL, proxy.URL)
	t.Setenv(EnvToken, b.token)
	var report strings.Builder
	finish, err := Start(context.Background(), b.dir, &report, nil)
	if err != nil || finish == nil {
		t.Fatalf("startup outage disabled retries: %v", err)
	}
	t.Cleanup(func() { _ = finish() })
	if !strings.Contains(report.String(), "will retry") {
		t.Fatal("startup outage did not report pending synchronization")
	}
	mustWrite(t, filepath.Join(a.dir, "settings.json"), `{"recovered":true}`)
	if err := a.SyncFiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	available.Store(true)
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(20 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("background reconciliation never recovered")
		case <-poll.C:
			data, err := readLocal(filepath.Join(b.dir, "settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			if string(data) == `{"recovered":true}` {
				if err := finish(); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
}

// TestStartRetriesPreferenceFailure ensures a recoverable startup conflict does
// not permanently disable background reconciliation or the normal-exit flush.
func TestStartRetriesPreferenceFailure(t *testing.T) {
	a, b, _ := testClients(t)
	ctx := context.Background()
	mustWrite(t, filepath.Join(a.dir, "settings.json"), `{"base":true}`)
	if err := a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(a.dir, "settings.json"), `{"a":true}`)
	mustWrite(t, filepath.Join(b.dir, "settings.json"), `{"b":true}`)
	if err := a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMPIG_BROKER_URL", b.base)
	t.Setenv("OMPIG_BROKER_TOKEN", b.token)
	finish, err := Start(ctx, b.dir, io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
	if finish == nil {
		t.Fatal("startup preference conflict permanently disabled synchronization")
	}
	t.Cleanup(func() { _ = finish() })
	mustWrite(t, filepath.Join(b.dir, "settings.json"), `{"a":true}`)
	if err := finish(); err != nil {
		t.Fatal(err)
	}
}
