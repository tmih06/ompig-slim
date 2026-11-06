package codingagent

// Behavioral tests for catalog metadata enrichment through the registry: a models.json entry's own
// int/tps/description win, a missing value is filled from the catalog or from a bundled declaration,
// and enrichment never crosses a model identity. Mirrors the composer/enrichment cases of
// packages/coding-agent/test/model-metadata.test.ts at the boundaries that stay uncertain in the port.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelmetadata"
)

// writeModelsJSON writes a models.json with one provider of one model and returns its path.
func writeModelsJSON(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write models.json: %v", err)
	}
	return path
}

// catalogRows is a small catalog with the deploy model's family row.
const catalogRows = `[
	  {"provider":"deepseek","id":"deepseek-flash","name":"DeepSeek V4.1 Flash 1017","int":32.6,"tps":118.4,
	   "reasoning":true,"cost":{"input":0.15,"output":0.6,"cacheRead":0.003},
	   "contextWindow":1048576,"maxTokens":393216,
	   "efforts":["low","high","max"]}
	]`

// metadataServiceWith installs a metadata service holding catalogRows.
func metadataServiceWith(t *testing.T, rows string) *modelmetadata.Service {
	t.Helper()
	snapshot, ok := ai.ParseModelCatalogMetadataSnapshot([]byte(`{"version":1,"fetchedAt":1,"checkedAt":1,"rows":` + rows + `}`))
	if ok {
		store := modelmetadata.NewInMemoryStore()
		if err := store.Write(context.Background(), snapshot); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
		service := modelmetadata.New(modelmetadata.Options{Store: store, Now: time.Now})
		if err := service.Load(context.Background()); err != nil {
			t.Fatalf("load: %v", err)
		}
		return service
	}
	// The fixture is a wire array, so parse it as a payload instead.
	parsed := ai.ParseModelCatalogMetadata([]byte(rows))
	store := modelmetadata.NewInMemoryStore()
	_ = store.Write(context.Background(), ai.ModelCatalogMetadataSnapshot{Version: 1, FetchedAt: 1, CheckedAt: 1, Rows: parsed})
	service := modelmetadata.New(modelmetadata.Options{Store: store, Now: time.Now})
	if err := service.Load(context.Background()); err != nil {
		t.Fatalf("load: %v", err)
	}
	return service
}

func TestRegistryEnrichmentFillsMissingMetrics(t *testing.T) {
	path := writeModelsJSON(t, `{"providers":{"deploy":{
	  "api":"openai-completions","baseUrl":"https://api-sf1.deploy.com/api/v1","apiKey":"test-key",
	  "models":[{"id":"deepseek-ai/deepseek-v4.1-flash-1017","name":"Deepseek 4.1 Flash"}]}}}`)
	registry := NewModelRegistryWithModelsPath(path)
	registry.SetModelMetadata(metadataServiceWith(t, catalogRows))

	models := registry.GetProviderModelData("deploy")
	if len(models) != 1 {
		t.Fatalf("models = %d, want 1", len(models))
	}
	model := models[0]
	if model.Int == nil || *model.Int != 32.6 {
		t.Errorf("int = %v", model.Int)
	}
	if model.TPS == nil || *model.TPS != 118.4 {
		t.Errorf("tps = %v", model.TPS)
	}
	if model.Capabilities.InputCostPer1M != 0.15 || model.Capabilities.OutputCostPer1M != 0.6 {
		t.Errorf("cost = %+v", model.Capabilities)
	}
	if model.Capabilities.ContextWindow != 1048576 || model.Capabilities.MaxOutputTokens != 393216 {
		t.Errorf("limits = %d/%d", model.Capabilities.ContextWindow, model.Capabilities.MaxOutputTokens)
	}
	if model.ProviderMeta.Reasoning != true {
		t.Error("an id-only custom entry did not inherit reasoning")
	}
	if model.ThinkingLevelMap[ai.ThinkingLow] == nil || model.ThinkingLevelMap[ai.ThinkingMedium] != nil {
		t.Errorf("thinking map = %v", model.ThinkingLevelMap)
	}
	if model.Catalog == nil || model.Catalog.Source != "cached" || model.Catalog.Provider != "deepseek" {
		t.Errorf("provenance = %+v", model.Catalog)
	}
}

func TestRegistryEnrichmentKeepsExplicitMetrics(t *testing.T) {
	path := writeModelsJSON(t, `{"providers":{"deploy":{
	  "api":"openai-completions","baseUrl":"https://api-sf1.deploy.com/api/v1","apiKey":"test-key",
	  "models":[{"id":"deepseek-ai/deepseek-v4.1-flash-1017","int":0,"description":"Fixture blurb",
	             "cost":{"input":1,"output":2,"cacheRead":0.5,"cacheWrite":0.25},
	             "contextWindow":1000000,"maxTokens":32768,
	             "thinkingLevelMap":{"low":"low"}}]}}}`)
	registry := NewModelRegistryWithModelsPath(path)
	registry.SetModelMetadata(metadataServiceWith(t, catalogRows))

	model := registry.GetProviderModelData("deploy")[0]
	// An explicit 0 is a real score and must survive enrichment.
	if model.Int == nil || *model.Int != 0 {
		t.Errorf("int = %v, want the explicit 0", model.Int)
	}
	// tps is exclusiveMinimum 0 in models.json, so a zero speed cannot be pinned; the catalog value is
	// inherited instead of being replaced by an invalid 0.
	if model.TPS == nil || *model.TPS != 118.4 {
		t.Errorf("tps = %v, want the inherited catalog speed", model.TPS)
	}
	if model.Description != "Fixture blurb" {
		t.Errorf("description = %q", model.Description)
	}
	if model.Capabilities.InputCostPer1M != 1 || model.Capabilities.OutputCostPer1M != 2 {
		t.Errorf("cost = %+v", model.Capabilities)
	}
	if model.Capabilities.ContextWindow != 1000000 || model.Capabilities.MaxOutputTokens != 32768 {
		t.Errorf("limits = %d/%d", model.Capabilities.ContextWindow, model.Capabilities.MaxOutputTokens)
	}
	if len(model.ThinkingLevelMap) != 1 || model.ThinkingLevelMap[ai.ThinkingLow] == nil {
		t.Errorf("thinking map = %v", model.ThinkingLevelMap)
	}
}

func TestRegistryEnrichmentIgnoresUnknownFamilies(t *testing.T) {
	path := writeModelsJSON(t, `{"providers":{"deploy":{
	  "api":"openai-completions","baseUrl":"https://api-sf1.deploy.com/api/v1","apiKey":"test-key",
	  "models":[{"id":"internal-embedder-v3"}]}}}`)
	registry := NewModelRegistryWithModelsPath(path)
	registry.SetModelMetadata(metadataServiceWith(t, catalogRows))

	model := registry.GetProviderModelData("deploy")[0]
	if model.Int != nil || model.TPS != nil || model.Description != "" {
		t.Errorf("an unrelated model was enriched: %+v", model)
	}
	if model.Catalog != nil {
		t.Errorf("provenance = %+v", model.Catalog)
	}
}

func TestRegistryEnrichmentNeverCrossesVersions(t *testing.T) {
	path := writeModelsJSON(t, `{"providers":{"deploy":{
	  "api":"openai-completions","baseUrl":"https://api-sf1.deploy.com/api/v1","apiKey":"test-key",
	  "models":[{"id":"deepseek-ai/deepseek-v4.2-flash"}]}}}`)
	registry := NewModelRegistryWithModelsPath(path)
	registry.SetModelMetadata(metadataServiceWith(t, catalogRows))

	model := registry.GetProviderModelData("deploy")[0]
	if model.Int != nil {
		t.Errorf("a different version was enriched with int %v", *model.Int)
	}
	if model.Catalog != nil {
		t.Errorf("provenance = %+v", model.Catalog)
	}
}

func TestRegistryEnrichmentThroughResolveEntry(t *testing.T) {
	// The request-auth path rebuilds an entry from a model; the metrics must survive that round trip.
	path := writeModelsJSON(t, `{"providers":{"deploy":{
	  "api":"openai-completions","baseUrl":"https://api-sf1.deploy.com/api/v1","apiKey":"test-key",
	  "models":[{"id":"deepseek-ai/deepseek-v4.1-flash-1017"}]}}}`)
	registry := NewModelRegistryWithModelsPath(path)
	registry.SetModelMetadata(metadataServiceWith(t, catalogRows))

	entry, ok := registry.Resolve("deploy", "deepseek-ai/deepseek-v4.1-flash-1017")
	if !ok {
		t.Fatal("model did not resolve")
	}
	if entry.Int == nil || *entry.Int != 32.6 {
		t.Errorf("entry int = %v", entry.Int)
	}
	if entry.TPS == nil || *entry.TPS != 118.4 {
		t.Errorf("entry tps = %v", entry.TPS)
	}
}

func TestRegistryFirstListingUsesPersistedSnapshotOffline(t *testing.T) {
	// model-runtime.ts:329-330 restores the durable snapshot before the runtime is constructed, so a
	// registry's very first listing is enriched even with the network unavailable.
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	if err := os.WriteFile(path, []byte(`{"providers":{"deploy":{
	  "api":"openai-completions","baseUrl":"https://api-sf1.deploy.com/api/v1","apiKey":"test-key",
	  "models":[{"id":"deepseek-ai/deepseek-v4.1-flash-1017"}]}}}`), 0o600); err != nil {
		t.Fatalf("write models.json: %v", err)
	}
	store := modelmetadata.NewFileStore(filepath.Join(dir, modelmetadata.FileName))
	if err := store.Write(context.Background(), ai.ModelCatalogMetadataSnapshot{
		Version:   ai.ModelCatalogMetadataVersion,
		FetchedAt: 1,
		CheckedAt: 1,
		Rows:      ai.ParseModelCatalogMetadata([]byte(catalogRows)),
	}); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	registry := NewModelRegistryWithModelsPath(path)
	// A fresh service over the same file stands in for a process start; Load runs synchronously.
	service := modelmetadata.New(modelmetadata.Options{Store: store, URL: "http://127.0.0.1:1/models.json"})
	registry.SetModelMetadata(service)
	if err := registry.LoadModelMetadata(context.Background()); err != nil {
		t.Fatalf("load: %v", err)
	}

	models := registry.GetProviderModelData("deploy")
	if len(models) != 1 {
		t.Fatalf("models = %d, want 1", len(models))
	}
	if models[0].Int == nil || *models[0].Int != 32.6 {
		t.Errorf("first listing int = %v, want the persisted catalog score", models[0].Int)
	}
	if models[0].TPS == nil || *models[0].TPS != 118.4 {
		t.Errorf("first listing tps = %v", models[0].TPS)
	}
}

func TestRegistryEnrichmentIsMemoizedPerGeneration(t *testing.T) {
	path := writeModelsJSON(t, `{"providers":{"deploy":{
	  "api":"openai-completions","baseUrl":"https://api-sf1.deploy.com/api/v1","apiKey":"test-key",
	  "models":[{"id":"deepseek-ai/deepseek-v4.1-flash-1017"}]}}}`)
	registry := NewModelRegistryWithModelsPath(path)
	service := metadataServiceWith(t, catalogRows)
	registry.SetModelMetadata(service)

	// A source list that a provider hands out repeatedly is enriched once per generation and returned
	// as the same slice, so a steady-state read allocates nothing.
	source := []*ai.Model{metricSourceModel()}
	first := registry.enrichModelData(source)
	second := registry.enrichModelData(source)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("models = %d then %d", len(first), len(second))
	}
	if &first[0] != &second[0] {
		t.Error("a repeated read at the same generation rebuilt the enriched list")
	}
	if first[0].Int == nil || *first[0].Int != 32.6 {
		t.Errorf("int = %v", first[0].Int)
	}

	// A different generation invalidates the memo: the same source list is enriched again.
	store := modelmetadata.NewInMemoryStore()
	if err := store.Write(context.Background(), ai.ModelCatalogMetadataSnapshot{
		Version: ai.ModelCatalogMetadataVersion, FetchedAt: 2, CheckedAt: 2,
		Rows: ai.ParseModelCatalogMetadata([]byte(`[{"provider":"deepseek","id":"deepseek-flash","name":"DeepSeek V4.1 Flash 1017","int":40}]`)),
	}); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	refreshed := modelmetadata.New(modelmetadata.Options{Store: store})
	if err := refreshed.Load(context.Background()); err != nil {
		t.Fatalf("load refreshed service: %v", err)
	}
	registry.SetModelMetadata(refreshed)
	third := registry.enrichModelData(source)
	if len(third) != 1 {
		t.Fatalf("models after the new generation = %d", len(third))
	}
	if third[0].Int == nil || *third[0].Int != 40 {
		t.Errorf("after the new generation int = %v, want 40", third[0].Int)
	}
	if third[0] == first[0] {
		t.Error("a stale memoized model survived a generation change")
	}
}

func TestSetModelMetadataInvalidatesMemoAtSameGeneration(t *testing.T) {
	path := writeModelsJSON(t, `{"providers":{"deploy":{
	  "api":"openai-completions","baseUrl":"https://api-sf1.deploy.com/api/v1","apiKey":"test-key",
	  "models":[{"id":"deepseek-ai/deepseek-v4.1-flash-1017"}]}}}`)
	registry := NewModelRegistryWithModelsPath(path)
	first := metadataServiceWith(t, catalogRows)
	registry.SetModelMetadata(first)

	source := []*ai.Model{metricSourceModel()}
	enriched := registry.enrichModelData(source)
	if len(enriched) != 1 || enriched[0].Int == nil || *enriched[0].Int != 32.6 {
		t.Fatalf("first enrichment = %+v", enriched)
	}

	// A different service whose rows differ but whose generation is also 1: the replacement API must
	// not keep serving enrichment resolved against the previous catalog.
	store := modelmetadata.NewInMemoryStore()
	if err := store.Write(context.Background(), ai.ModelCatalogMetadataSnapshot{
		Version: ai.ModelCatalogMetadataVersion, FetchedAt: 1, CheckedAt: 1,
		Rows: ai.ParseModelCatalogMetadata([]byte(`[{"provider":"deepseek","id":"deepseek-flash","name":"DeepSeek V4.1 Flash 1017","int":55}]`)),
	}); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	replacement := modelmetadata.New(modelmetadata.Options{Store: store})
	if err := replacement.Load(context.Background()); err != nil {
		t.Fatalf("load replacement: %v", err)
	}
	if replacement.Generation() != first.Generation() {
		t.Fatalf("fixture generations differ: %d and %d", replacement.Generation(), first.Generation())
	}
	registry.SetModelMetadata(replacement)

	// The same source list must be re-resolved: the memo of the previous service is gone.
	swapped := registry.enrichModelData(source)
	if len(swapped) != 1 {
		t.Fatalf("models after the swap = %d", len(swapped))
	}
	if swapped[0].Int == nil || *swapped[0].Int != 55 {
		t.Errorf("int after the swap = %v, want 55", swapped[0].Int)
	}
	if &swapped[0] == &enriched[0] {
		t.Error("the memoized list of the replaced service survived the swap")
	}
}

// metricSourceModel is the base model a provider hands to enrichment.
func metricSourceModel() *ai.Model {
	return &ai.Model{
		ID:           "deepseek-ai/deepseek-v4.1-flash-1017",
		DisplayName:  "Deepseek 4.1 Flash",
		ProviderMeta: ai.ProviderMetadata{ProviderID: "deploy", API: ai.APIOpenAICompletions},
		Input:        []string{"text"},
		Capabilities: ai.ModelCapabilities{ContextWindow: 128000, MaxOutputTokens: 16384},
	}
}

func TestRegistryMetadataRefreshKeepsRowsOnFailure(t *testing.T) {
	registry := NewModelRegistry(t.TempDir())
	service := modelmetadata.New(modelmetadata.Options{
		Store: modelmetadata.NewInMemoryStore(),
		URL:   "http://127.0.0.1:1/models.json",
	})
	registry.SetModelMetadata(service)
	// An unreachable origin must not fail the call or erase rows.
	result, err := registry.RefreshModelMetadata(context.Background(), true, true)
	if err != nil {
		t.Fatalf("refresh returned an error: %v", err)
	}
	if !result.Stale {
		t.Errorf("unreachable origin did not mark the rows stale: %+v", result)
	}
	if !strings.Contains(result.Error, "metadata catalog") && result.Error == "" {
		t.Errorf("no failure reason: %+v", result)
	}
	if _, refreshed := registry.ModelMetadataResult(); !refreshed {
		t.Error("the outcome was not recorded")
	}
}

func TestRegistryDefinitionDisplayName(t *testing.T) {
	if got := (modelDefinition{ID: "m", Name: "Name"}).DisplayName(); got != "Name" {
		t.Errorf("DisplayName = %q", got)
	}
	if got := (modelDefinition{ID: "m"}).DisplayName(); got != "m" {
		t.Errorf("DisplayName without a name = %q", got)
	}
}
