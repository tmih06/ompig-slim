package broker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestRemoteMetadataCountsAccountsNotProviders preserves metadata semantics
// when count/existence reads use the broker's direct aggregate endpoints.
// Every call still queries live host state, including after logout.
func TestRemoteMetadataCountsAccountsNotProviders(t *testing.T) {
	client, _, _ := testClients(t)
	store := &remoteStore{client: client}
	ctx := context.Background()
	for _, key := range []string{"one", "two"} {
		if _, err := store.SaveAccount(ctx, "fixture", &key, ai.Credential{Type: ai.CredentialAPIKey, Key: key}); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := store.AccountCount(); err != nil || count != 2 {
		t.Fatal("metadata collapsed accounts into providers", count, err)
	}
	if present, err := store.HasProvider(ctx, "fixture"); err != nil || !present {
		t.Fatal("stored provider missing", err)
	}
	if present, err := store.HasProvider(ctx, "missing"); err != nil || present {
		t.Fatal("invented provider", err)
	}
	if err := store.Delete(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	if count, err := store.AccountCount(); err != nil || count != 0 {
		t.Fatal("metadata reused a stale snapshot", count, err)
	}
	if present, err := store.HasProvider(ctx, "fixture"); err != nil || present {
		t.Fatal("logged-out provider still present", err)
	}
}

// TestRemoteMetadataRejectsInvalidSnapshot keeps metadata decode failures from
// exposing partially decoded account counts or silently reporting absence.
func TestRemoteMetadataRejectsInvalidSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"providerId":"fixture","type":"api_key"},{"providerId":false}]`))
	}))
	defer server.Close()
	store := &remoteStore{client: &Client{base: server.URL, http: server.Client()}}
	if count, err := store.AccountCount(); err == nil || count != 0 {
		t.Fatal("invalid snapshot produced a partial count", count, err)
	}
	if present, err := store.HasProvider(context.Background(), "fixture"); err == nil || present {
		t.Fatal("invalid snapshot produced provider metadata", present, err)
	}
}
