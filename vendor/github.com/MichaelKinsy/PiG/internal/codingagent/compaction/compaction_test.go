package compaction

import "testing"

// TestShouldCompactUsesEightyPercentOfTheModelWindow keeps the default policy
// tied to the model's advertised window rather than a fixed reserve amount.
func TestShouldCompactUsesEightyPercentOfTheModelWindow(t *testing.T) {
	settings := DefaultCompactionSettings
	if ShouldCompact(79_999, 100_000, settings) {
		t.Fatal("compaction triggered below the 80% threshold")
	}
	if !ShouldCompact(80_000, 100_000, settings) {
		t.Fatal("compaction did not trigger at the 80% threshold")
	}
	settings.TriggerTokens = 90_000
	if ShouldCompact(85_000, 100_000, settings) {
		t.Fatal("explicit trigger was ignored")
	}
}
