package cli

import (
	"slices"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestStoredAccountKeysPreservesProviderOrderAndNullIdentity guards the shared
// use/logout diagnostic, including the distinction between nil and empty keys.
func TestStoredAccountKeysPreservesProviderOrderAndNullIdentity(t *testing.T) {
	accounts := []ai.CredentialAccountInfo{
		{ProviderID: "fixture", AccountKey: new("second")},
		{ProviderID: "other", AccountKey: new("excluded")},
		{ProviderID: "fixture"},
		{ProviderID: "fixture", AccountKey: new("")},
		{ProviderID: "fixture", AccountKey: new("first")},
	}
	want := []string{"second", "<no identity>", "", "first"}
	if got := storedAccountKeys(accounts, "fixture"); !slices.Equal(got, want) {
		t.Fatalf("keys = %q, want %q", got, want)
	}
	if got := storedAccountKeys(accounts, "missing"); len(got) != 0 {
		t.Fatalf("unknown provider has keys: %q", got)
	}
}
