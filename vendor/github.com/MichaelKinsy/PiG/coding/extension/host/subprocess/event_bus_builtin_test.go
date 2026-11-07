package subprocess

import (
	"reflect"
	"testing"
)

func TestBuiltinBusSnapshotAndNestedReply(t *testing.T) {
	h := &Host{}
	var calls []string
	var off func()
	first := h.On("request", func(value any) {
		calls = append(calls, "first")
		off()
		h.Emit("reply", value)
	})
	off = h.On("request", func(any) { calls = append(calls, "second") })
	reply := h.On("reply", func(value any) {
		if value.(map[string]any)["version"] != float64(2) {
			t.Fatal(value)
		}
		calls = append(calls, "reply")
	})
	h.Emit("request", map[string]int{"version": 2})
	h.Emit("request", map[string]int{"version": 2})
	first()
	first()
	reply()
	h.Emit("request", nil)
	if !reflect.DeepEqual(calls, []string{"first", "reply", "second", "first", "reply"}) {
		t.Fatal(calls)
	}
}
