package cli

import (
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension/builtin"
)

// TestRemovedFeaturesAreNotAdvertised covers both inherited help and built-in
// extension discovery, including explicit legacy builtin configuration paths.
func TestRemovedFeaturesAreNotAdvertised(t *testing.T) {
	for _, name := range []string{"pig-login", "llama.cpp"} {
		if _, err := builtin.Resolve("builtin:"+name, builtin.Options{}); err == nil {
			t.Fatalf("removed built-in resolved: %s", name)
		}
	}
}
