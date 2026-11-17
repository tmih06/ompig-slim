package broker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestBrokerSessionDistributionAndRestart(t *testing.T) {
	for _, provider := range []string{"codex-json", "mistral", "custom-provider"} {
		t.Run(provider, func(t *testing.T) { testBrokerSessionDistributionAndRestart(t, provider) })
	}
}

func testBrokerSessionDistributionAndRestart(t *testing.T, provider string) {
	dir := t.TempDir()
	host, err := NewHandler(dir, "fixture-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { host.Close() })
	server := httptest.NewServer(host)
	t.Cleanup(server.Close)
	client := func() *remoteStore {
		return &remoteStore{client: &Client{base: server.URL, token: "fixture-secret", dir: t.TempDir(), http: server.Client()}}
	}
	stores := []*remoteStore{client(), client()}
	for _, key := range []string{"a", "b"} {
		if _, err := stores[0].SaveAccount(t.Context(), provider, &key, ai.Credential{Type: ai.CredentialOAuth, Access: "fixture-" + key, Refresh: "private-" + key, Expires: time.Now().Add(time.Hour).UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	model := &ai.Model{ID: "session-fixture", ProviderMeta: ai.ProviderMetadata{ProviderID: provider}}
	attempt := func(t *testing.T, store *remoteStore, session string) string {
		t.Helper()
		var selected string
		_, err := ai.NewAccountRouter(store).ImagesRoutes(t.Context(), model, ai.AccountRouteRequest{SessionID: session}, func(_ context.Context, selection ai.AccountSelection, _ ai.AccountObserve, _ bool) (ai.AssistantImages, error) {
			if selection.AccountKey != nil {
				selected = *selection.AccountKey
			}
			return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return selected
	}
	for _, test := range []struct{ session, want string }{{"one", "b"}, {"two", "a"}, {"one", "b"}, {"two", "a"}} {
		if got := attempt(t, stores[len(test.session)%2], test.session); got != test.want {
			t.Fatalf("remote session %s used %s, want %s", test.session, got, test.want)
		}
	}
	server.Close()
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	host, err = NewHandler(dir, "fixture-secret")
	if err != nil {
		t.Fatal(err)
	}
	server = httptest.NewServer(host)
	t.Cleanup(server.Close)
	stores = []*remoteStore{client(), client()}
	if got := attempt(t, stores[0], "two"); got != "a" {
		t.Fatalf("broker restart lost pin: %s", got)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[string]int{}
	for i := range 12 {
		wg.Go(func() {
			route := ai.AccountSessionRoute{SessionID: fmt.Sprintf("new-%d", i), ModelID: model.ID}
			selection, err := stores[i%2].ResolveSessionAccount(t.Context(), provider, route, true)
			if err != nil || selection == nil || selection.AccountKey == nil {
				t.Errorf("remote assignment failed: %v", err)
				return
			}
			mu.Lock()
			counts[*selection.AccountKey]++
			mu.Unlock()
		})
	}
	wg.Wait()
	if counts["a"] != 6 || counts["b"] != 6 {
		t.Fatalf("broker clients did not share a cursor: %v", counts)
	}
	for _, store := range stores {
		if _, err := os.Stat(filepath.Join(store.client.dir, "agent.db")); !os.IsNotExist(err) {
			t.Fatal("session assignment created a client credential database", err)
		}
	}
	for _, body := range []string{
		`{"operation":"session-account","provider":"codex-json"}`,
		`{"operation":"session-account","provider":"codex-json","route":{"sessionId":"","modelId":"m"}}`,
		`{"operation":"session-account","provider":"","route":{"sessionId":"s","modelId":"m"}}`,
		`{"operation":"session-account","provider":"custom-provider","route":{"sessionId":"s","modelId":""}}`,
	} {
		request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/credential", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer fixture-secret")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid session route returned %d", response.StatusCode)
		}
	}
}
