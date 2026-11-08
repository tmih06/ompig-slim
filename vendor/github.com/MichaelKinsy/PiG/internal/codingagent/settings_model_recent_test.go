package codingagent

import (
	"reflect"
	"testing"
)

// TestModelMRUIsBoundedAndDetached never copies project-only model usage globally.
func TestModelMRUIsBoundedAndDetached(t *testing.T) {
	sm := NewInMemorySettingsManager(Settings{})
	for _, selector := range []string{"p/a", "p/b", "p/a"} {
		if err := sm.RecordModelUsage(selector); err != nil {
			t.Fatal(err)
		}
	}
	if got := sm.Get().ModelRecent; !reflect.DeepEqual(got, []string{"p/a", "p/b"}) {
		t.Fatal(got)
	}
	copy := sm.Get()
	copy.ModelRecent[0] = "changed"
	if sm.Get().ModelRecent[0] != "p/a" {
		t.Fatal("MRU shares mutable settings")
	}
}
