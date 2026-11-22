package modelmetadata

import (
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestPostVariantWordsStayDistinguishing protects base models from inheriting
// benchmarks of a different variant built on the same family: lite and vl are
// identity words, not effort lanes, so they may not be dropped after the
// variant word.
func TestPostVariantWordsStayDistinguishing(t *testing.T) {
	lite, vl := 11.7, 13.6
	rows := []ai.ModelCatalogMetadataRow{
		{Provider: "google", ID: "gemini-3.5-flash-lite", Int: &lite},
		{Provider: "nvidia", ID: "nemotron-3.5-super-vl", Int: &vl},
	}
	if got := MatchCatalogMetadataRows("gemini-3.5-flash", "Gemini 3.5 Flash", rows); len(got) != 0 {
		t.Fatalf("gemini-3.5-flash borrowed flash-lite row: %v", got)
	}
	if got := MatchCatalogMetadataRows("nemotron-3.5-super", "Nemotron 3.5 Super", rows); len(got) != 0 {
		t.Fatalf("nemotron-3.5-super borrowed super-vl row: %v", got)
	}
	if got := MatchCatalogMetadataRows("gemini-3.5-flash-lite", "Gemini 3.5 Flash Lite", rows); len(got) != 1 {
		t.Fatal("flash-lite no longer matches itself", got)
	}
	if got := MatchCatalogMetadataRows("nemotron-3.5-super-vl", "Nemotron 3.5 Super VL", rows); len(got) != 1 {
		t.Fatal("super-vl no longer matches itself", got)
	}
}

// TestEffortLanesShareBaseScore keeps the lane words transparent: an effort or
// service tier is the same model at a different setting, so it still inherits
// the family's published benchmark.
func TestEffortLanesShareBaseScore(t *testing.T) {
	score := 41.2
	rows := []ai.ModelCatalogMetadataRow{{Provider: "openai", ID: "gpt-6-sol", Int: &score}}
	for _, id := range []string{"gpt-6-sol-high", "gpt-6-sol-max", "gpt-6-sol-high-priority"} {
		if got := MatchCatalogMetadataRows(id, id, rows); len(got) != 1 {
			t.Fatalf("%s lost its base score", id)
		}
	}
}
