package modelroles

import "testing"

// TestAvailableLiteralIDsWinRouting keeps exact qualified/unqualified @ IDs
// ahead of upstream interpretation while still resolving routed role defaults.
func TestAvailableLiteralIDsWinRouting(t *testing.T) {
	pool := []Candidate{{Provider: "openrouter", ID: "literal@default", Kind: "chat"}, {Provider: "openrouter", ID: "other", Kind: "chat"}}
	config := Config{}
	for _, selector := range []string{"literal@default", "openrouter/literal@default"} {
		resolved, err := config.ResolveAvailable(selector, "", pool)
		if err != nil || resolved != "openrouter/literal@default" {
			t.Fatal(resolved, err)
		}
	}
	resolved, err := config.ResolveAvailable("openrouter/other@fireworks:high", "", pool)
	if err != nil || resolved != "openrouter/other@fireworks:high" {
		t.Fatal(resolved, err)
	}
}
