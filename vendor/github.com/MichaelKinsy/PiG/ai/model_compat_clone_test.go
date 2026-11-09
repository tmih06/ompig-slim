package ai

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

// serializedCompatClone is the previous copy operation, retained as an oracle
// for the fast path's normalization, error and opaque-extension semantics.
func serializedCompatClone(in *ModelCompat) *ModelCompat {
	if in == nil {
		return nil
	}
	data, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out ModelCompat
	if json.Unmarshal(data, &out) != nil {
		return nil
	}
	return &out
}

// TestCompatCloneMatchesSerialization traverses every imported row, not merely
// scalar fixtures. Catalog compat extensions frequently contain nested RawJSON.
func TestCompatCloneMatchesSerialization(t *testing.T) {
	for _, provider := range OMPCatalogProviders() {
		rows, _ := OMPChatCatalog(provider)
		for _, row := range rows {
			got, want := cloneCompat(row.Compat), serializedCompatClone(row.Compat)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("clone changed %s/%s", provider, row.ID)
			}
		}
	}
	for _, input := range []*ModelCompat{
		nil, {},
		{Extra: map[string]json.RawMessage{"boolOn": json.RawMessage(` true `), "boolOff": json.RawMessage("\nfalse\t"), "nothing": json.RawMessage(` null `)}},
		{Extra: map[string]json.RawMessage{"nested": json.RawMessage(` { "html": "<script>", "array": [null, 1.0, true] } `), "null": json.RawMessage(`null`)}},
		{SupportsStore: new(false), Extra: map[string]json.RawMessage{"nested": json.RawMessage(` { "html": "<script>", "array": [null, 1.0, true] } `), "nil": nil}},
		{Extra: map[string]json.RawMessage{"supportsStore": json.RawMessage(`true`)}},
		{Extra: map[string]json.RawMessage{"SUPPORTSSTORE": json.RawMessage(`true`)}},
		{Extra: map[string]json.RawMessage{"ſupportsStore": json.RawMessage(`true`)}},
		{Extra: map[string]json.RawMessage{"\xff": json.RawMessage(`1`)}},
		{Extra: map[string]json.RawMessage{"broken": json.RawMessage(`{"x":`)}},
		{VLLMPriority: new(math.NaN())},
		{AllowedFallbackModels: []AnthropicAllowedFallbackModel{}},
		{ChatTemplateArgs: map[string]any{"nested": map[string]any{"on": true}}},
	} {
		if got, want := cloneCompat(input), serializedCompatClone(input); !reflect.DeepEqual(got, want) {
			t.Fatalf("normalization changed: input=%+v got=%+v want=%+v", input, got, want)
		}
	}
}

// FuzzCompatCloneMatchesSerialization covers user-authored opaque keys/values,
// including malformed JSON and Unicode. Size caps keep each normalization probe
// bounded; the reference is the existing serializer, not the optimized clone.
func FuzzCompatCloneMatchesSerialization(f *testing.F) {
	for _, seed := range []struct{ key, value string }{{"futureFlag", "true"}, {"futureFlag", " false "}, {"supportsStore", "false"}, {"SUPPORTSSTORE", "true"}, {"ſupportsStore", "false"}, {"nested", `{"x":[null,1.0,"<script>"]}`}, {"broken", `{"x":`}, {"empty", ""}} {
		f.Add(seed.key, []byte(seed.value))
	}
	f.Fuzz(func(t *testing.T, key string, raw []byte) {
		if len(key) > 128 || len(raw) > 4096 {
			return
		}
		input := &ModelCompat{SupportsStore: new(true), Extra: map[string]json.RawMessage{key: raw}}
		if got, want := cloneCompat(input), serializedCompatClone(input); !reflect.DeepEqual(got, want) {
			t.Fatalf("clone changed key=%q raw=%q", key, raw)
		}
	})
}

// TestCompatCloneDetachesExtensions prevents model-local mutations from
// poisoning the immutable catalog or another concurrent request's descriptor.
func TestCompatCloneDetachesExtensions(t *testing.T) {
	input := &ModelCompat{SupportsStore: new(true), Extra: map[string]json.RawMessage{"custom": json.RawMessage(`{"flag":true}`)}}
	cloned := cloneCompat(input)
	if cloned == nil {
		t.Fatal("missing clone")
	}
	*cloned.SupportsStore = false
	cloned.Extra["custom"][0] = '['
	cloned.Extra["another"] = json.RawMessage(`1`)
	if !*input.SupportsStore || string(input.Extra["custom"]) != `{"flag":true}` || len(input.Extra) != 1 {
		t.Fatal("clone aliases source", input)
	}
}
