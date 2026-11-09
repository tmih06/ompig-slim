package modelmetadata

import (
	"github.com/MichaelKinsy/PiG/ai"
	"testing"
)

// TestProTierNeverBorrowsBaseScore protects paid pro variants from inheriting
// lower-cost base benchmarks when a second variant word follows the family.
func TestProTierNeverBorrowsBaseScore(t *testing.T) {
	score := 23.8
	rows := []ai.ModelCatalogMetadataRow{{Provider: "openai", ID: "gpt-5.6-luna", Int: &score}}
	if got := MatchCatalogMetadataRows("gpt-5.6-luna-pro", "GPT-5.6 Luna Pro", rows); len(got) != 0 {
		t.Fatal("pro matched base", got)
	}
	if got := MatchCatalogMetadataRows("gpt-5.6-luna", "GPT-5.6 Luna", rows); len(got) != 1 {
		t.Fatal("base no longer matches", got)
	}
}
