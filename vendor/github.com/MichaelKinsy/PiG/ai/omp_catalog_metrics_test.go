package ai

import "testing"

// TestOMPDateReleaseUsesProviderMetricsAndCatalogThroughput covers OMP's dated
// release alias: provider-owned intelligence wins, while missing throughput may
// come from another provider's measurement for the same base model.
func TestOMPDateReleaseUsesProviderMetricsAndCatalogThroughput(t *testing.T) {
	generated, ok := LookupModelExact("openai/gpt-4o-2024-11-20")
	if !ok {
		t.Fatal("missing dated release")
	}
	model := generated.ToModel()
	if model.Int == nil || *model.Int != 8.4 {
		t.Fatalf("provider score=%v", model.Int)
	}
	if model.TPS == nil || *model.TPS != 117.3 {
		t.Fatalf("shared throughput=%v", model.TPS)
	}
	if model.Catalog == nil || model.Catalog.Source != "bundled" {
		t.Fatal("missing metric provenance", model.Catalog)
	}
}

// TestOMPExactMetricPairsKeepCredentialScopedScoresLocal verifies that shared
// public measurements travel together while account-authoritative observations
// cannot overwrite another provider\'s bundled score.
func TestOMPExactMetricPairsKeepCredentialScopedScoresLocal(t *testing.T) {
	for _, test := range []struct {
		id    string
		score float64
		speed *float64
	}{{"gpt-5.2", 43.3, new(71.7)}, {"gpt-5.6-sol", 47, nil}, {"gpt-5.3-codex", 32.5, nil}, {"gpt-4o-2024-08-06", 7.7, nil}} {
		generated, ok := LookupModelExact("openai/" + test.id)
		if !ok {
			t.Fatal(test.id)
		}
		model := generated.ToModel()
		if model.Int == nil || *model.Int != test.score {
			t.Fatal(test.id, "score", model.Int)
		}
		if (model.TPS == nil) != (test.speed == nil) {
			t.Fatal(test.id, "speed", model.TPS)
		}
		if model.TPS != nil && *model.TPS != *test.speed {
			t.Fatal(test.id, "speed", *model.TPS)
		}
	}
}

// TestOMPMetricAliasDoesNotCrossDistinctVariants prevents date handling from
// stripping ordinary model variants such as mini, pro, or a different family.
func TestOMPMetricAliasDoesNotCrossDistinctVariants(t *testing.T) {
	for _, id := range []string{"gpt-4o-mini-2024-11-20", "gpt-4o-pro-2024-11-20"} {
		if got := stripCatalogReleaseDate(id); got == "gpt-4o" {
			t.Fatal(id, "collapsed to another model")
		}
	}
	if got := stripCatalogReleaseDate("gpt-4o-2024-11-20"); got != "gpt-4o" {
		t.Fatal(got)
	}
	if got := stripCatalogReleaseDate("gpt-4o-20241120"); got != "gpt-4o" {
		t.Fatal(got)
	}
}
