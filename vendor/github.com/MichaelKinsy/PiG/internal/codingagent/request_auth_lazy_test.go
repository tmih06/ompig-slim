package codingagent

import (
	"sync"
	"testing"
)

// TestRequestAuthMetadataDoesNotLoadModels keeps provider/auth discovery
// separate from catalog reads; listing a provider still returns detached rows.
func TestRequestAuthMetadataDoesNotLoadModels(t *testing.T) {
	loads := 0
	provider := &RuntimeProvider{ID: "fixture", Name: "Fixture", models: sync.OnceValue(func() []RuntimeModel {
		loads++
		return []RuntimeModel{{Provider: "fixture", ID: "model"}}
	})}
	runtime := &RequestAuthRuntime{providers: []*RuntimeProvider{provider}, providerByID: map[string]*RuntimeProvider{"fixture": provider}}
	if len(runtime.GetProviders()) != 1 || runtime.GetProvider("fixture") == nil || loads != 0 {
		t.Fatal("provider discovery loaded the catalog", loads)
	}
	models := runtime.GetModels("fixture")
	if len(models) != 1 || loads != 1 {
		t.Fatal(models, loads)
	}
	models[0].ID = "caller mutation"
	if models = runtime.GetModels("fixture"); len(models) != 1 || models[0].ID != "model" || loads != 1 {
		t.Fatal("catalog was reloaded or exposed to caller mutation", models, loads)
	}
}
