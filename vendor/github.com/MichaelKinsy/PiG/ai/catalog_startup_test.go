package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

// The fast snapshot path must retain the validating decoder's row rejection,
// null handling, number conversion and last-key-wins behavior on hand-edited files.
func TestCatalogSnapshotFastPathMatchesValidation(t *testing.T) {
	for _, rows := range []string{
		`[{"provider":"openai","id":"gpt-5","contextWindow":128000.0,"maxTokens":10.9,"efforts":["none","high"],"input":["text","image"],"tps":12}]`,
		`[{"provider":"x","id":"a","name":null,"input":[],"efforts":["none"],"reasoning":null,"supportsOffEffort":null,"cost":{"input":-1}}]`,
		`[{"provider":"x","id":"first","tps":0},{"provider":"x","id":"good","int":2},{"provider":"x","id":null}]`,
		`[{"provider":"x","id":"bad","int":"wrong"},{"provider":"x","id":"good"}]`,
		`[{"provider":"x","id":"old","id":"new"}]`,
		`[{"Provider":"x","id":"missing-case-sensitive-provider"}]`,
		`[{"provider":"x","id":"bad-input","input":[null]},{"provider":"x","id":"bad-effort","efforts":["unknown"]}]`,
	} {
		t.Run(rows, func(t *testing.T) {
			payload := []byte(`{"version":1,"fetchedAt":1,"checkedAt":2,"rows":` + rows + `}`)
			var reference struct {
				Rows []any `json:"rows"`
			}
			decoder := json.NewDecoder(bytes.NewReader(payload))
			decoder.UseNumber()
			if err := decoder.Decode(&reference); err != nil {
				t.Fatal(err)
			}
			want := []ModelCatalogMetadataRow{}
			for _, value := range reference.Rows {
				if row, ok := decodeStoredCatalogRow(value); ok {
					want = append(want, row)
				}
			}
			got, ok := ParseModelCatalogMetadataSnapshot(payload)
			if !ok || !reflect.DeepEqual(got.Rows, want) {
				t.Fatalf("snapshot rows = %#v, ok=%v; want %#v", got.Rows, ok, want)
			}
		})
	}
}

func TestLazyCatalogShapePreservesFirstEditAndConcurrentEncoding(t *testing.T) {
	raw := []byte(`{"provider":"fixture","id":"chat","contextWindow":128000.0,"cost":{"input":1.50,"output":2,"unknown":{"keep":true}},"future":[1,2]}`)
	model, known, err := DecodeStoredModel(raw)
	if err != nil || !known {
		t.Fatalf("decode: known=%v err=%v", known, err)
	}
	unchanged, err := EncodeStoredModel(model)
	if err != nil || string(unchanged) != string(raw) {
		t.Fatalf("round trip = %s, %v", unchanged, err)
	}
	// A separate model is edited before its shape or baseline has been built.
	model, _, err = DecodeStoredModel(raw)
	if err != nil {
		t.Fatal(err)
	}
	model.(*Model).Capabilities.InputCostPer1M = 9
	var tasks sync.WaitGroup
	for range 8 {
		tasks.Go(func() {
			encoded, err := EncodeStoredModel(model)
			if err != nil {
				t.Error(err)
				return
			}
			var got map[string]any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Error(err)
				return
			}
			cost := got["cost"].(map[string]any)
			unknown, _ := cost["unknown"].(map[string]any)
			future, _ := got["future"].([]any)
			if cost["input"] != float64(9) || unknown["keep"] != true || len(future) != 2 {
				t.Errorf("first edit or unknown fields lost: %s", encoded)
			}
		})
	}
	tasks.Wait()
}

func TestFileCatalogSnapshotsDoNotShareMutableModels(t *testing.T) {
	ctx := context.Background()
	store := NewFileModelsStore(filepath.Join(t.TempDir(), "models-store.json"))
	model, _, err := DecodeStoredModel([]byte(`{"provider":"fixture","id":"a","contextWindow":12,"cost":{"input":1},"future":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Write(ctx, "fixture", ModelsStoreEntry{Models: []AnyModel{model}}); err != nil {
		t.Fatal(err)
	}
	first, err := ReadModelsStoreSnapshot(ctx, store, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	first.Models[0].(*Model).Capabilities.InputCostPer1M = 99
	second, err := ReadModelsStoreSnapshot(ctx, store, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if second.Models[0].(*Model).Capabilities.InputCostPer1M != 1 {
		t.Fatal("snapshot mutation reached the store")
	}
}
