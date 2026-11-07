package yamljson

import (
	"testing"

	"github.com/MichaelKinsy/PiG/internal/yaml12"
)

// TestMarshalSymbols preserves the bridge's symbol handling: null at the root
// and in arrays, but omitted from object properties, including nested arrays.
func TestMarshalSymbols(t *testing.T) {
	symbol := yaml12.Symbol("fixture")
	object := &yaml12.Object{
		Keys: []string{"hidden", "items"},
		Values: map[string]any{
			"hidden": symbol,
			"items":  []any{symbol, []any{symbol}, "kept"},
		},
	}
	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{"root", symbol, "null"},
		{"array", []any{symbol, "kept"}, `[null,"kept"]`},
		{"object", object, `{"items":[null,[null],"kept"]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := Marshal(test.value)
			if err != nil || string(got) != test.want {
				t.Fatalf("Marshal = %s, %v; want %s", got, err, test.want)
			}
		})
	}
}
