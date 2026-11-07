package ai

import "testing"

// TestUsageFormEncodingKeepsSortedWireBytes fixes key order and form escaping
// independently of the map's iteration order.
func TestUsageFormEncodingKeepsSortedWireBytes(t *testing.T) {
	fields := map[string]string{"z": "last", "a b": "x+y &/雪", "empty": ""}
	const want = "a+b=x%2By+%26%2F%E9%9B%AA&empty=&z=last"
	for range 10 {
		if got := urlValues(fields); got != want {
			t.Fatalf("form = %q, want %q", got, want)
		}
	}
	if got := urlValues(nil); got != "" {
		t.Fatalf("empty form = %q", got)
	}
}

// TestCursorUnifiedPoolUsesLargestSignal preserves optional-field precedence,
// over-limit fractions, and the nonnegative floor of the unified weekly pool.
func TestCursorUnifiedPoolUsesLargestSignal(t *testing.T) {
	for _, test := range []struct {
		name   string
		fields map[string]any
		want   float64
	}{
		{"missing signals", map[string]any{}, 0},
		{"percent only", map[string]any{"totalPercentUsed": 25.0}, 0.25},
		{"used only", map[string]any{"used": 500.0}, 0.5},
		{"percent wins", map[string]any{"totalPercentUsed": 75.0, "used": 250.0}, 0.75},
		{"used wins", map[string]any{"totalPercentUsed": 25.0, "used": 750.0}, 0.75},
		{"negative signals", map[string]any{"totalPercentUsed": -25.0, "used": -500.0}, 0},
		{"over limit", map[string]any{"totalPercentUsed": 150.0}, 1.5},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.fields["limit"] = 1000.0
			got := parseCursorUnifiedPool(test.fields)
			if got == nil || got.UsedFraction == nil || *got.UsedFraction != test.want {
				t.Fatalf("pool = %+v, want used fraction %v", got, test.want)
			}
			if *got.Used > *got.Limit || *got.Remaining < 0 {
				t.Fatalf("absolute pool amount is out of bounds: %+v", got)
			}
		})
	}
}
