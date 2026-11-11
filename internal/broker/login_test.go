package broker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestLoginPersistsLocalConnection verifies the two typed prompts, private
// persistence, environment-free reconnect and exclusion from shared documents.
func TestLoginPersistsLocalConnection(t *testing.T) {
	a, _, server := testClients(t)
	t.Setenv("OMPIG_BROKER_URL", "")
	t.Setenv("OMPIG_BROKER_TOKEN", "")
	previous := ai.CredentialStoreFactory
	t.Cleanup(func() { ai.CredentialStoreFactory = previous })
	var prompts []ai.AuthPrompt
	interaction := ai.AuthInteraction{Prompt: func(ctx context.Context, prompt ai.AuthPrompt) (string, error) {
		prompts = append(prompts, prompt)
		switch prompt.(type) {
		case ai.AuthTextPrompt:
			return server.URL + "/", nil
		case ai.AuthSecretPrompt:
			return "sync-secret", nil
		default:
			t.Fatal("unexpected prompt")
			return "", nil
		}
	}}
	if err := Login(context.Background(), a.dir, interaction); err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 2 || prompts[0].Type() != "text" || prompts[1].Type() != "secret" {
		t.Fatal("expected URL and masked secret prompts")
	}
	info, err := os.Stat(filepath.Join(a.dir, connectionFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("connection permissions %o", info.Mode().Perm())
	}
	saved, err := loadConnection(a.dir)
	if err != nil || saved.URL != server.URL || saved.Secret != "sync-secret" {
		t.Fatal("saved connection lost", err)
	}
	// Verification must not upload the sync secret or start credential sync.
	empty, err := (&remoteStore{client: a}).snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatal("login unexpectedly changed hosted credentials")
	}
	if slices.Contains(SharedFiles, connectionFile) {
		t.Fatal("bootstrap secret would be uploaded")
	}
	connected, err := Connect(context.Background(), a.dir)
	if err != nil || connected == nil {
		t.Fatal("saved login did not reconnect", err)
	}
	if err = a.request(context.Background(), "GET", "/v1/items/"+connectionFile, "", nil, nil); err == nil {
		t.Fatal("connection secret exposed as shared file")
	}
}

// TestLoginFailurePreservesConnection pins failure/cancellation semantics: no
// partial save and no accidental destruction of a working local connection.
func TestLoginFailurePreservesConnection(t *testing.T) {
	for _, failure := range []string{"cancel", "unauthorized", "invalid-url"} {
		t.Run(failure, func(t *testing.T) {
			a, _, server := testClients(t)
			original := `{"url":"https://previous.example","secret":"previous-secret"}`
			mustWrite(t, filepath.Join(a.dir, connectionFile), original)
			cancelled := errors.New("cancelled by user")
			interaction := ai.AuthInteraction{Prompt: func(ctx context.Context, prompt ai.AuthPrompt) (string, error) {
				if prompt.Type() == "text" {
					if failure == "invalid-url" {
						return "https://example.com/not-an-origin", nil
					}
					return server.URL, nil
				}
				if failure == "cancel" {
					return "", cancelled
				}
				return "wrong-secret", nil
			}}
			err := Login(context.Background(), a.dir, interaction)
			if err == nil {
				t.Fatal("accepted failed login")
			}
			if failure == "cancel" && !errors.Is(err, cancelled) {
				t.Fatal("lost cancellation")
			}
			data, _ := os.ReadFile(filepath.Join(a.dir, connectionFile))
			if string(data) != original {
				t.Fatal("overwrote previous connection")
			}
		})
	}
}

// TestConnectionEnvironmentPrecedence keeps explicit environment destinations
// from borrowing a saved secret intended for a different service.
func TestConnectionEnvironmentPrecedence(t *testing.T) {
	dir := t.TempDir()
	data, _ := json.Marshal(connection{URL: "https://saved.example", Secret: "saved-secret"})
	mustWrite(t, filepath.Join(dir, connectionFile), string(data))
	t.Setenv("OMPIG_BROKER_URL", "https://override.example")
	t.Setenv("OMPIG_BROKER_TOKEN", "")
	saved, err := loadConnection(dir)
	if err != nil {
		t.Fatal(err)
	}
	if saved.URL != "https://override.example" || saved.Secret != "" {
		t.Fatal("environment borrowed saved credentials")
	}
	if _, err = newClient(dir, saved); err == nil {
		t.Fatal("missing override token accepted")
	}
	t.Setenv("OMPIG_BROKER_URL", "")
	t.Setenv("OMPIG_BROKER_TOKEN", "rotated-secret")
	saved, err = loadConnection(dir)
	if err != nil || saved.URL != "https://saved.example" || saved.Secret != "rotated-secret" {
		t.Fatal("token rotation override failed", err)
	}
}
