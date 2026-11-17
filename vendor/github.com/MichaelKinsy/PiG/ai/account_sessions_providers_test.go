package ai_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestSessionAssignmentsAcrossProviders(t *testing.T) {
	for _, test := range []struct {
		provider string
		kind     ai.CredentialType
	}{
		{"anthropic", ai.CredentialAPIKey},
		{"mistral", ai.CredentialOAuth},
		{"google-antigravity", ai.CredentialOAuth},
		{"cursor", ai.CredentialOAuth},
		{"openai-codex", ai.CredentialOAuth},
		{"custom-provider", ai.CredentialAPIKey},
	} {
		t.Run(test.provider, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "agent.db")
			store := sessionStore(t, path)
			for _, key := range []string{"a", "b"} {
				credential := ai.Credential{Type: test.kind, Key: "fixture-" + key, Access: codexToken("plus")}
				if _, err := store.SaveAccount(t.Context(), test.provider, &key, credential); err != nil {
					t.Fatal(err)
				}
			}
			attempt := func(store ai.CredentialStore, session, want string) {
				t.Helper()
				var selected *string
				_, err := ai.NewAccountRouter(store).ClassifyRoutes(t.Context(), okModel(test.provider, "session-fixture"), ai.AccountRouteRequest{SessionID: session}, func(_ context.Context, selection ai.AccountSelection, _ ai.AccountObserve, _ bool) (ai.ClassifierResult, error) {
					selected = selection.AccountKey
					return ai.ClassifierResult{StopReason: ai.ClassifierStopReasonStop}, nil
				})
				if err != nil || selected == nil || *selected != want {
					t.Fatalf("%s session %s selected %v, want %s: %v", test.provider, session, selected, want, err)
				}
			}
			attempt(store, "one", "b")
			attempt(store, "two", "a")
			attempt(store, "one", "b")
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store = sessionStore(t, path)
			attempt(store, "two", "a")
			attempt(ai.NewRuntimeCredentials(store), "three", "b")
			if err := store.BlockAccount(t.Context(), test.provider, keyOf("a"), "auth", time.Now().Add(time.Hour).UnixMilli()); err != nil {
				t.Fatal(err)
			}
			attempt(store, "two", "b")
			if err := store.ClearAccountBlocks(t.Context(), test.provider, keyOf("a"), nil); err != nil {
				t.Fatal(err)
			}
			attempt(store, "two", "b")
			if _, err := store.DeleteAccount(t.Context(), test.provider, "b"); err != nil {
				t.Fatal(err)
			}
			attempt(store, "one", "a")
		})
	}
}

func TestSessionAssignmentsAreProviderScoped(t *testing.T) {
	store := sessionStore(t, filepath.Join(t.TempDir(), "agent.db"))
	for _, provider := range []string{"mistral", "custom-provider"} {
		for _, key := range []string{"a", "b"} {
			if _, err := store.SaveAccount(t.Context(), provider, &key, ai.Credential{Type: ai.CredentialAPIKey, Key: "fixture-" + key}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, test := range []struct{ provider, session, want string }{
		{"mistral", "one", "b"},
		{"mistral", "two", "a"},
		{"custom-provider", "two", "b"},
		{"custom-provider", "one", "a"},
	} {
		selection, err := store.ResolveSessionAccount(t.Context(), test.provider, ai.AccountSessionRoute{SessionID: test.session, ModelID: "same-model"}, true)
		if err != nil || selection == nil || selection.AccountKey == nil || *selection.AccountKey != test.want {
			t.Fatalf("providers shared a cursor or assignment: %v %v", selection, err)
		}
	}
}
