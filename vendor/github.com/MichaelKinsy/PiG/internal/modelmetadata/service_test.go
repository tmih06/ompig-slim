package modelmetadata

// Behavioral tests for the uncertain boundaries of the port: the wire parser's pruning and trust
// boundary, identity parsing and matching (no cross-model borrowing), ranking and consensus, the
// cache lifecycle (TTL, failure backoff, ETag 304, offline), and patch application against explicit
// values. Mirrors packages/coding-agent/test/model-metadata.test.ts.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// catalogPayload is a minimal catalog in the stencil wire shape.
func catalogPayload() string {
	return `{
	  "deepseek": {
	    "id": "deepseek",
	    "api": "https://api.deepseek.com",
	    "models": {
	      "deepseek-flash": {
	        "id": "deepseek-flash",
	        "name": "DeepSeek V4.1 Flash 1017",
	        "int": 32.6,
	        "cost": {"input": 0.15, "output": 0.6, "cache_read": 0.003},
	        "limit": {"context": 1048576, "output": 393216},
	        "modalities": {"input": ["text", "image"]},
	        "reasoning": true,
	        "reasoning_options": [{"type": "effort", "values": ["low", "high", "max"]}]
	      },
	      "deepseek-v4-pro": {
	        "id": "deepseek-v4-pro",
	        "name": "DeepSeek V4 Pro",
	        "int": 23.1,
	        "cost": {"input": 0.66, "output": 1.98, "cache_read": 0.022},
	        "limit": {"context": 1048576, "output": 393216}
	      }
	    }
	  },
	  "openrouter": {
	    "models": {
	      "deepseek/deepseek-v4.1-flash": {
	        "id": "deepseek/deepseek-v4.1-flash",
	        "int": 32.6,
	        "tps": 118.4,
	        "cost": {"input": 0.15, "output": 0.6, "cache_read": 0.003},
	        "limit": {"context": 1048576, "output": 393216}
	      },
	      "openai/gpt-5.4": {
	        "id": "openai/gpt-5.4",
	        "int": 61,
	        "cost": {"input": 2.5, "output": 15},
	        "limit": {"context": 1050000, "output": 128000},
	        "reasoning_options": [{"type": "effort", "values": ["none", "low", "high"]}]
	      }
	    }
	  }
	}`
}

func payloadRows(t *testing.T) []ai.ModelCatalogMetadataRow {
	t.Helper()
	return ai.ParseModelCatalogMetadata([]byte(catalogPayload()))
}

func TestParsePrunesWireRows(t *testing.T) {
	rows := payloadRows(t)
	var flash *ai.ModelCatalogMetadataRow
	for i := range rows {
		if rows[i].Provider == "deepseek" && rows[i].ID == "deepseek-flash" {
			flash = &rows[i]
		}
	}
	if flash == nil {
		t.Fatalf("deepseek-flash row missing from %d rows", len(rows))
	}
	if flash.Name != "DeepSeek V4.1 Flash 1017" {
		t.Errorf("name = %q", flash.Name)
	}
	if flash.Int == nil || *flash.Int != 32.6 {
		t.Errorf("int = %v", flash.Int)
	}
	if got := strings.Join(flash.Efforts, ","); got != "low,high,max" {
		t.Errorf("efforts = %q", got)
	}
	if flash.SupportsOffEffort {
		t.Errorf("supportsOffEffort = true without a published none")
	}
	if flash.Cost == nil || flash.Cost.Input == nil || *flash.Cost.Input != 0.15 {
		t.Errorf("cost = %+v", flash.Cost)
	}
	if flash.Cost.CacheRead == nil || *flash.Cost.CacheRead != 0.003 {
		t.Errorf("cache read = %v", flash.Cost.CacheRead)
	}
	if flash.ContextWindow == nil || *flash.ContextWindow != 1048576 {
		t.Errorf("contextWindow = %v", flash.ContextWindow)
	}
	if flash.MaxTokens == nil || *flash.MaxTokens != 393216 {
		t.Errorf("maxTokens = %v", flash.MaxTokens)
	}
	if got := strings.Join(flash.Input, ","); got != "text,image" {
		t.Errorf("input = %q", got)
	}
}

func TestParseSkipsInvalidRows(t *testing.T) {
	rows := ai.ParseModelCatalogMetadata([]byte(`{"deepseek":{"models":{"bad":{"int":"not a number"},"good":{"int":1},"nameless":{"name":"Only A Name"}}}}`))
	if len(rows) != 1 || rows[0].ID != "good" {
		t.Fatalf("rows = %+v, want only the valid one", rows)
	}
}

func TestParseSnapshotValidatesEveryRow(t *testing.T) {
	payload := `{"version":1,"fetchedAt":1,"checkedAt":2,"rows":[{"provider":"deepseek","id":"good","int":1},{"provider":"deepseek"},"junk"]}`
	snapshot, ok := ai.ParseModelCatalogMetadataSnapshot([]byte(payload))
	if !ok {
		t.Fatal("snapshot rejected")
	}
	if len(snapshot.Rows) != 1 || snapshot.Rows[0].ID != "good" {
		t.Errorf("rows = %+v", snapshot.Rows)
	}
	if _, ok := ai.ParseModelCatalogMetadataSnapshot([]byte(`{"version":999,"rows":[]}`)); ok {
		t.Error("a snapshot of another version was accepted")
	}
	if _, ok := ai.ParseModelCatalogMetadataSnapshot([]byte(`{"version":1`)); ok {
		t.Error("a truncated snapshot was accepted")
	}
}

func TestParseOffEffortAndEffortValidation(t *testing.T) {
	rows := ai.ParseModelCatalogMetadata([]byte(`{"p":{"models":{"m":{"reasoning_options":[{"type":"effort","values":["none","low","high"]},{"type":"budget","values":["max"]}]}}}}`))
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	if !rows[0].SupportsOffEffort {
		t.Error(`"none" did not set supportsOffEffort`)
	}
	if got := strings.Join(rows[0].Efforts, ","); got != "low,high" {
		t.Errorf("efforts = %q", got)
	}
	// A persisted row naming an unknown tier is dropped rather than sent as a request value.
	snapshot, ok := ai.ParseModelCatalogMetadataSnapshot([]byte(`{"version":1,"fetchedAt":1,"checkedAt":2,"rows":[{"provider":"p","id":"m","efforts":["ultra"]}]}`))
	if !ok {
		t.Fatal("snapshot rejected")
	}
	if len(snapshot.Rows) != 0 {
		t.Errorf("rows = %+v, want the invalid row dropped", snapshot.Rows)
	}
}

func TestVariantKeyParsing(t *testing.T) {
	cases := []struct {
		value   string
		family  string
		version string
		variant string
	}{
		{"@cf/deepseek-ai/deepseek-v4.1-flash-1017", "deepseek", "4.1", "flash"},
		{"[Kiro] claude-opus-4-8", "claude", "4.8", "opus"},
		{"global.anthropic.claude-opus-4-8-v1:0", "claude", "4.8", "opus"},
		{"qwen3.6-plus", "qwen", "3.6", "plus"},
	}
	for _, testCase := range cases {
		key, ok := ParseVariantKey(testCase.value)
		if !ok {
			t.Errorf("%q did not parse", testCase.value)
			continue
		}
		if key.Family != testCase.family || key.Version != testCase.version || key.Variant != testCase.variant {
			t.Errorf("%q parsed as %+v", testCase.value, key)
		}
	}
}

func TestVariantKeyMoESizeAndDates(t *testing.T) {
	key, ok := ParseVariantKey("llama-4-35b-a3b-instruct-20250514")
	if !ok {
		t.Fatal("MoE id did not parse")
	}
	if key.Size != "35b-a3b" {
		t.Errorf("size = %q, want 35b-a3b", key.Size)
	}
	if key.Date != "20250514" {
		t.Errorf("date = %q", key.Date)
	}
	if !hasMarker(key, "instruct") {
		t.Errorf("markers = %v", key.Markers)
	}
}

func hasMarker(key *VariantKey, marker string) bool {
	for _, candidate := range key.Markers {
		if candidate == marker {
			return true
		}
	}
	return false
}

func TestVariantKeysMatchWildcardsMissingDate(t *testing.T) {
	base := func(date string) *VariantKey {
		return &VariantKey{Family: "deepseek", Version: "4.1", Variant: "flash", HasVersion: true, HasVariant: true, Date: date, HasDate: date != ""}
	}
	if !VariantKeysMatch(base("1017"), base("")) {
		t.Error("a missing date was not wildcarded")
	}
	if VariantKeysMatch(base("1017"), base("0801")) {
		t.Error("two release dates matched")
	}
}

func TestMatchNeverBorrowsAnotherVariantOrVersion(t *testing.T) {
	rows := payloadRows(t)
	if row, ok := FindCatalogMetadataRow("deepseek-ai/deepseek-v4.1-flash-1017", "Deepseek 4.1 Flash", rows); !ok || row.ID != "deepseek-flash" {
		t.Errorf("deploy id matched %+v", row)
	}
	if row, ok := FindCatalogMetadataRow("deepseek-v4.1-flash-1017", "", rows); !ok || row.ID != "deepseek-flash" {
		t.Errorf("dated id without a name matched %+v", row)
	}
	if row, ok := FindCatalogMetadataRow("deepseek-v4.2-flash", "", rows); ok {
		t.Errorf("a different version matched %+v", row)
	}
	if row, ok := FindCatalogMetadataRow("deepseek-v4-pro", "", rows); !ok || row.ID != "deepseek-v4-pro" {
		t.Errorf("the pro row matched %+v", row)
	}
	if row, ok := FindCatalogMetadataRow("deepseek-v4.1-pro", "", rows); ok {
		t.Errorf("a different version of pro matched %+v", row)
	}
	if row, ok := FindCatalogMetadataRow("gpt-5.4-pro", "", rows); ok {
		t.Errorf("a different variant matched %+v", row)
	}
	if row, ok := FindCatalogMetadataRow("totally-unknown-model", "", rows); ok {
		t.Errorf("an unknown family matched %+v", row)
	}
}

func TestConsensusRefusesDisagreement(t *testing.T) {
	rows := payloadRows(t)
	metrics := ResolveCatalogMetrics("deepseek-ai/deepseek-v4.1-flash-1017", "", rows)
	if metrics.Int == nil || *metrics.Int != 32.6 {
		t.Errorf("int = %v", metrics.Int)
	}
	if metrics.TPS == nil || *metrics.TPS != 118.4 {
		t.Errorf("tps = %v", metrics.TPS)
	}
	disagreeing := []ai.ModelCatalogMetadataRow{
		{Provider: "deepseek", ID: "deepseek-flash", Name: "DeepSeek V4.1 Flash 1017", Int: float64Pointer(32.6)},
		{Provider: "openrouter", ID: "deepseek-flash", Name: "DeepSeek V4.1 Flash 1017", Int: float64Pointer(33.1)},
	}
	if metrics := ResolveCatalogMetrics("deepseek-flash", "", disagreeing); metrics.Int != nil || metrics.TPS != nil {
		t.Errorf("disagreeing metrics = %+v, want none", metrics)
	}
}

func float64Pointer(value float64) *float64 { return new(value) }

func TestRankingPrefersCanonicalThenExactThenRicher(t *testing.T) {
	rows := []ai.ModelCatalogMetadataRow{
		{Provider: "openrouter", ID: "deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash", Int: float64Pointer(1)},
		{Provider: "deepseek", ID: "deepseek-flash", Name: "DeepSeek V4.1 Flash", Status: "beta"},
		{Provider: "deepseek", ID: "deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash", Int: float64Pointer(2)},
	}
	matches := MatchCatalogMetadataRows("deepseek-v4.1-flash", "", rows)
	if len(matches) != 3 {
		t.Fatalf("matched %d rows", len(matches))
	}
	// The family's own provider wins, and within it the exact id outranks a shorter name-matched id.
	if matches[0].Provider != "deepseek" || matches[0].ID != "deepseek-v4.1-flash" {
		t.Errorf("winner = %s/%s", matches[0].Provider, matches[0].ID)
	}
	if matches[1].ID != "deepseek-flash" || matches[2].Provider != "openrouter" {
		t.Errorf("order = %+v", matches)
	}
}

func TestStatusRankSortsDeprecatedLast(t *testing.T) {
	rows := []ai.ModelCatalogMetadataRow{
		{Provider: "deepseek", ID: "deepseek-flash", Name: "DeepSeek V4.1 Flash", Status: "deprecated"},
		{Provider: "deepseek", ID: "deepseek-flash", Name: "DeepSeek V4.1 Flash"},
	}
	matches := MatchCatalogMetadataRows("deepseek-flash", "", rows)
	if matches[0].Status != "" {
		t.Errorf("deprecated row ranked first: %+v", matches)
	}
}

// fakeCatalog is a test server that counts requests and can answer 304.
type fakeCatalog struct {
	server    *httptest.Server
	payload   string
	etag      string
	requests  atomic.Int64
	notMod    atomic.Bool
	fail      atomic.Bool
	lastIfNon atomic.Value
}

func newFakeCatalog(t *testing.T, payload, etag string) *fakeCatalog {
	t.Helper()
	catalog := &fakeCatalog{payload: payload, etag: etag}
	catalog.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		catalog.requests.Add(1)
		catalog.lastIfNon.Store(request.Header.Get("If-None-Match"))
		if catalog.fail.Load() {
			http.Error(writer, "boom", http.StatusInternalServerError)
			return
		}
		if catalog.notMod.Load() && request.Header.Get("If-None-Match") == catalog.etag {
			writer.WriteHeader(http.StatusNotModified)
			return
		}
		writer.Header().Set("ETag", catalog.etag)
		_, _ = writer.Write([]byte(catalog.payload))
	}))
	t.Cleanup(catalog.server.Close)
	return catalog
}

func testService(catalog *fakeCatalog, store Store, now *time.Time) *Service {
	service := New(Options{
		Store:  store,
		URL:    catalog.server.URL,
		Client: catalog.server.Client(),
		Now:    func() time.Time { return *now },
	})
	return service
}

func TestServiceFetchPersistAndRevalidate(t *testing.T) {
	catalog := newFakeCatalog(t, catalogPayload(), `"v1"`)
	store := NewInMemoryStore()
	now := time.UnixMilli(1_000_000)
	service := testService(catalog, store, &now)

	first, err := service.Refresh(context.Background(), RefreshOptions{Force: true})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if first.Source != SourceNetwork || first.Stale {
		t.Errorf("first refresh = %+v", first)
	}
	if len(first.Rows) == 0 {
		t.Error("no rows")
	}
	snapshot, err := store.Read(context.Background())
	if err != nil || snapshot == nil {
		t.Fatalf("snapshot = %v, %v", snapshot, err)
	}
	if snapshot.Version != ai.ModelCatalogMetadataVersion || snapshot.ETag != `"v1"` {
		t.Errorf("snapshot = %+v", snapshot)
	}

	// A second refresh inside the TTL does not touch the network.
	if _, err := service.Refresh(context.Background(), RefreshOptions{}); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if requests := catalog.requests.Load(); requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}

	// Past the TTL it revalidates; a 304 keeps the rows and only moves freshness.
	catalog.notMod.Store(true)
	now = now.Add(TTL + time.Millisecond)
	revalidated, err := service.Refresh(context.Background(), RefreshOptions{})
	if err != nil {
		t.Fatalf("revalidation: %v", err)
	}
	if revalidated.Source != SourceCached || revalidated.Stale {
		t.Errorf("revalidation = %+v", revalidated)
	}
	if len(revalidated.Rows) != len(first.Rows) {
		t.Errorf("rows changed across a 304: %d then %d", len(first.Rows), len(revalidated.Rows))
	}
	if sent, _ := catalog.lastIfNon.Load().(string); sent != `"v1"` {
		t.Errorf("If-None-Match = %q, want the stored etag", sent)
	}
}

func TestServiceKeepsRowsWhenNetworkFails(t *testing.T) {
	catalog := newFakeCatalog(t, catalogPayload(), `"v1"`)
	now := time.UnixMilli(1_000_000)
	service := testService(catalog, NewInMemoryStore(), &now)
	loaded, err := service.Refresh(context.Background(), RefreshOptions{Force: true})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	catalog.fail.Store(true)
	now = now.Add(TTL + time.Millisecond)
	failed, err := service.Refresh(context.Background(), RefreshOptions{})
	if err != nil {
		t.Fatalf("failed refresh returned an error: %v", err)
	}
	if !failed.Stale {
		t.Error("failed refresh did not mark the rows stale")
	}
	if failed.Error == "" {
		t.Error("failed refresh kept no error reason")
	}
	if len(failed.Rows) != len(loaded.Rows) {
		t.Errorf("rows erased by a failure: %d then %d", len(loaded.Rows), len(failed.Rows))
	}
	// The failure window suppresses immediate retries.
	before := catalog.requests.Load()
	if _, err := service.Refresh(context.Background(), RefreshOptions{}); err != nil {
		t.Fatalf("backoff refresh: %v", err)
	}
	if requests := catalog.requests.Load(); requests != before {
		t.Errorf("requests during the backoff window = %d, want %d", requests, before)
	}
}

func TestServiceOfflineKeepsDurableSnapshot(t *testing.T) {
	catalog := newFakeCatalog(t, catalogPayload(), `"v1"`)
	store := NewInMemoryStore()
	now := time.UnixMilli(1_000_000)
	snapshot, ok := ai.ParseModelCatalogMetadataSnapshot([]byte(`{"version":1,"fetchedAt":1000000,"checkedAt":1000000,"rows":[{"provider":"deepseek","id":"deepseek-flash","int":32.6}]}`))
	if !ok {
		t.Fatal("fixture snapshot rejected")
	}
	if err := store.Write(context.Background(), snapshot); err != nil {
		t.Fatalf("write: %v", err)
	}
	service := testService(catalog, store, &now)
	allowNetwork := false
	result, err := service.Refresh(context.Background(), RefreshOptions{AllowNetwork: &allowNetwork})
	if err != nil {
		t.Fatalf("offline refresh: %v", err)
	}
	if requests := catalog.requests.Load(); requests != 0 {
		t.Errorf("offline refresh made %d requests", requests)
	}
	if len(result.Rows) != 1 || result.Source != SourceCached {
		t.Errorf("offline result = %+v", result)
	}
}

func TestServiceSharedTransportCancelsWithLastWaiter(t *testing.T) {
	requestStarted := make(chan struct{}, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestStarted <- struct{}{}
		select {
		case <-release:
		case <-request.Context().Done():
			return
		}
		_, _ = writer.Write([]byte(catalogPayload()))
	}))
	defer server.Close()

	service := New(Options{Store: NewInMemoryStore(), URL: server.URL, Client: server.Client()})
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	var waiters sync.WaitGroup
	waiters.Add(2)
	var firstErr, secondErr error
	go func() {
		defer waiters.Done()
		_, firstErr = service.Refresh(firstCtx, RefreshOptions{Force: true})
	}()
	<-requestStarted
	go func() {
		defer waiters.Done()
		_, secondErr = service.Refresh(secondCtx, RefreshOptions{Force: true})
	}()
	// One caller leaving cancels only its own wait.
	cancelFirst()
	time.Sleep(10 * time.Millisecond)
	close(release)
	cancelSecond()
	waiters.Wait()
	if !errors.Is(firstErr, context.Canceled) {
		t.Errorf("first caller error = %v, want context.Canceled", firstErr)
	}
	// The second caller sees the shared refresh settle even though the first left.
	if secondErr != nil && !errors.Is(secondErr, context.Canceled) {
		t.Errorf("second caller error = %v", secondErr)
	}
}

func TestFileStoreRoundTripAndCorruptRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	fileStore := NewFileStore(path)
	snapshot := ai.ModelCatalogMetadataSnapshot{
		Version:   ai.ModelCatalogMetadataVersion,
		FetchedAt: 1,
		CheckedAt: 2,
		ETag:      `"abc"`,
		Rows:      []ai.ModelCatalogMetadataRow{{Provider: "deepseek", ID: "deepseek-flash", Int: float64Pointer(32.6)}},
	}
	if err := fileStore.Write(context.Background(), snapshot); err != nil {
		t.Fatalf("write: %v", err)
	}
	reread, err := NewFileStore(path).Read(context.Background())
	if err != nil || reread == nil {
		t.Fatalf("read = %v, %v", reread, err)
	}
	if len(reread.Rows) != 1 || reread.ETag != `"abc"` {
		t.Errorf("reread = %+v", reread)
	}

	// A corrupt file is an empty catalog, not a crash.
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if corrupt, err := NewFileStore(path).Read(context.Background()); err != nil || corrupt != nil {
		t.Errorf("corrupt read = %+v, %v", corrupt, err)
	}
}

func TestResolvePatchSkipsExplicitFields(t *testing.T) {
	catalog := newFakeCatalog(t, catalogPayload(), `"v1"`)
	now := time.UnixMilli(1_000_000)
	service := testService(catalog, NewInMemoryStore(), &now)
	if _, err := service.Refresh(context.Background(), RefreshOptions{Force: true}); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	patch, ok := service.ResolvePatch("deepseek-ai/deepseek-v4.1-flash-1017", "Deepseek 4.1 Flash", FieldSet{}, ResolveOptions{Mode: ModeFull, AllowEffortMap: true})
	if !ok {
		t.Fatal("no patch for the deploy model")
	}
	if patch.Source != SourceNetwork || patch.Provider != "deepseek" || patch.ModelID != "deepseek-flash" {
		t.Errorf("patch = %+v", patch)
	}
	if patch.Int == nil || *patch.Int != 32.6 {
		t.Errorf("int = %v", patch.Int)
	}
	if patch.TPS == nil || *patch.TPS != 118.4 {
		t.Errorf("tps = %v", patch.TPS)
	}
	if patch.Cost == nil || patch.Cost.Input == nil || *patch.Cost.Input != 0.15 {
		t.Errorf("cost = %+v", patch.Cost)
	}

	explicit := FieldSet{FieldCostInput: true, FieldCostOutput: true, FieldCostCacheRead: true}
	filtered, ok := service.ResolvePatch("deepseek-ai/deepseek-v4.1-flash-1017", "", explicit, ResolveOptions{Mode: ModeFull})
	if !ok {
		t.Fatal("no patch with explicit cost")
	}
	if filtered.Cost != nil {
		t.Errorf("cost = %+v, want none", filtered.Cost)
	}
	for _, field := range filtered.Fields {
		if strings.HasPrefix(string(field), "cost.") {
			t.Errorf("patch filled %q despite the explicit pin", field)
		}
	}
}

func TestResolvePatchDoesNotCrossReleaseDates(t *testing.T) {
	catalog := newFakeCatalog(t, `[{"provider":"deepseek","id":"deepseek-flash","name":"DeepSeek V4.1 Flash 0801","int":20}]`, `"v1"`)
	now := time.UnixMilli(1_000_000)
	service := testService(catalog, NewInMemoryStore(), &now)
	if _, err := service.Refresh(context.Background(), RefreshOptions{Force: true}); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if patch, ok := service.ResolvePatch("deepseek-ai/deepseek-v4.1-flash-1017", "", FieldSet{}, ResolveOptions{Mode: ModeFull}); ok {
		t.Errorf("another release date produced %+v", patch)
	}
}

func TestApplyMetadataPatchKeepsExplicitValues(t *testing.T) {
	description := "Catalog blurb"
	model := PatchableModel{
		Cost:          ai.ModelCost{},
		ContextWindow: 1_000_000,
		MaxTokens:     32_768,
	}
	patched, changed := ApplyMetadataPatch(model, Patch{
		Source:      SourceNetwork,
		Provider:    "deepseek",
		ModelID:     "deepseek-flash",
		Int:         float64Pointer(32.6),
		Description: &description,
		Cost:        &PatchCost{Input: float64Pointer(0.15), Output: float64Pointer(0.6), CacheRead: float64Pointer(0.003)},
	}, false)
	if !changed {
		t.Fatal("patch changed nothing")
	}
	if patched.Cost.Input != 0.15 || patched.Cost.Output != 0.6 || patched.Cost.CacheRead != 0.003 {
		t.Errorf("cost = %+v", patched.Cost)
	}
	if patched.Int == nil || *patched.Int != 32.6 {
		t.Errorf("int = %v", patched.Int)
	}
	if patched.Description == nil || *patched.Description != "Catalog blurb" {
		t.Errorf("description = %v", patched.Description)
	}
	if patched.Catalog == nil || patched.Catalog.Source != "network" || patched.Catalog.Provider != "deepseek" {
		t.Errorf("provenance = %+v", patched.Catalog)
	}

	// A later metrics-only pass keeps the earlier field list.
	patched, _ = ApplyMetadataPatch(patched, Patch{Source: SourceNetwork, Provider: "deepseek", ModelID: "deepseek-flash", TPS: float64Pointer(118.4)}, false)
	if patched.Catalog == nil {
		t.Fatal("no provenance after the second pass")
	}
	fields := strings.Join(patched.Catalog.Fields, " ")
	for _, want := range []string{"int", "description", "cost.input", "tps"} {
		if !strings.Contains(fields, want) {
			t.Errorf("fields %q lost %q", fields, want)
		}
	}
}

func TestApplyMetadataPatchLeavesIdentityAlone(t *testing.T) {
	patched, changed := ApplyMetadataPatch(PatchableModel{}, Patch{Source: SourceNetwork}, false)
	if changed {
		t.Error("an empty patch reported a change")
	}
	if patched.Catalog != nil {
		t.Errorf("provenance = %+v", patched.Catalog)
	}
}

func TestModelSendsEffort(t *testing.T) {
	cases := []struct {
		api       string
		supports  *bool
		wantsSend bool
	}{
		{"openai-completions", nil, true},
		{"openai-completions", boolPointer(false), false},
		{"openai-responses", boolPointer(false), false},
		{"anthropic-messages", nil, true},
		{"google-generative-ai", nil, true},
		{"something-else", nil, false},
	}
	for _, testCase := range cases {
		if got := ModelSendsEffort(testCase.api, testCase.supports); got != testCase.wantsSend {
			t.Errorf("ModelSendsEffort(%q, %v) = %v", testCase.api, testCase.supports, got)
		}
	}
}

func boolPointer(value bool) *bool { return new(value) }

func TestEffortMapAndInheritanceLadder(t *testing.T) {
	row := ai.ModelCatalogMetadataRow{Efforts: []string{"low", "high"}, SupportsOffEffort: true}
	ladder := EffortMapFromCatalog(row)
	if ladder[ai.ThinkingMinimal] != nil || ladder[ai.ThinkingMedium] != nil || ladder[ai.ThinkingXHigh] != nil {
		t.Errorf("unsupported tiers got a value: %v", ladder)
	}
	if ladder[ai.ThinkingLow] == nil || *ladder[ai.ThinkingLow] != "low" {
		t.Errorf("low = %v", ladder[ai.ThinkingLow])
	}
	if ladder[ai.ThinkingOff] == nil || *ladder[ai.ThinkingOff] != "none" {
		t.Errorf("off = %v", ladder[ai.ThinkingOff])
	}

	// An inherited bundled ladder drops the host's own off spelling.
	off := "none"
	maxLevel := "max"
	inherited := InheritanceLadder(ai.ThinkingLevelMap{ai.ThinkingOff: &off, ai.ThinkingMax: &maxLevel})
	if _, present := inherited[ai.ThinkingOff]; present {
		t.Errorf("inherited ladder kept off: %v", inherited)
	}
	if inherited[ai.ThinkingMax] == nil || *inherited[ai.ThinkingMax] != "max" {
		t.Errorf("max = %v", inherited[ai.ThinkingMax])
	}
}

func TestJSONShapeOfSnapshot(t *testing.T) {
	// A persisted snapshot keeps the upstream member names, so an existing file stays readable.
	snapshot := ai.ModelCatalogMetadataSnapshot{
		Version:   ai.ModelCatalogMetadataVersion,
		FetchedAt: 1,
		CheckedAt: 2,
		Rows:      []ai.ModelCatalogMetadataRow{{Provider: "p", ID: "m", SupportsOffEffort: true, ContextWindow: intPointer(1000)}},
	}
	encoded, err := ai.MarshalModelCatalogMetadataSnapshot(snapshot)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, member := range []string{"version", "fetchedAt", "checkedAt", "rows"} {
		if _, present := decoded[member]; !present {
			t.Errorf("snapshot JSON lacks %q: %s", member, encoded)
		}
	}
	if strings.Contains(string(encoded), "ETag") {
		t.Errorf("snapshot JSON wrote the Go field name: %s", encoded)
	}
}

func intPointer(value int) *int { return new(value) }

func TestStoreRevisionSkipsRepeatedReads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models-metadata.json")
	store := NewFileStore(path)
	snapshot := ai.ModelCatalogMetadataSnapshot{Version: 1, FetchedAt: 1, CheckedAt: 2, Rows: []ai.ModelCatalogMetadataRow{{Provider: "p", ID: "m", Int: float64Pointer(1)}}}
	if err := store.Write(context.Background(), snapshot); err != nil {
		t.Fatalf("write: %v", err)
	}
	first, err := store.Read(context.Background())
	if err != nil || first == nil {
		t.Fatalf("read = %v, %v", first, err)
	}
	// Mutating the returned rows must not corrupt the cached copy.
	first.Rows[0].Provider = "mutated"
	second, err := store.Read(context.Background())
	if err != nil || second == nil {
		t.Fatalf("second read = %v, %v", second, err)
	}
	if second.Rows[0].Provider != "p" {
		t.Errorf("cached rows were mutated: %+v", second.Rows[0])
	}
}

func TestServiceRequestTimeoutIsBounded(t *testing.T) {
	// A server that never answers must not hold the refresh past the attempt timeout.
	release := make(chan struct{})
	defer close(release)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-release:
		case <-request.Context().Done():
		}
	}))
	defer server.Close()
	service := New(Options{Store: NewInMemoryStore(), URL: server.URL, Client: server.Client()})
	started := time.Now()
	result, err := service.Refresh(context.Background(), RefreshOptions{Force: true})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if elapsed > 10*time.Second {
		t.Errorf("refresh waited %v, want about the 4s attempt timeout", elapsed)
	}
	if !result.Stale || result.Error == "" {
		t.Errorf("timed-out refresh = %+v", result)
	}
}

func TestParseProviderMapIgnoresScalarMetadata(t *testing.T) {
	rows := ai.ParseModelCatalogMetadata([]byte(`{"deepseek":{"id":"deepseek","env":["DEEPSEEK_API_KEY"],"models":{"m":{"int":1}}}}`))
	if len(rows) != 1 || rows[0].Provider != "deepseek" || rows[0].ID != "m" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestParseArrayPayloadUsesRowProvider(t *testing.T) {
	rows := ai.ParseModelCatalogMetadata([]byte(`[{"provider":"a","id":"m","int":1},{"provider":"b","id":"n","tps":2}]`))
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Provider != "a" || rows[1].Provider != "b" {
		t.Errorf("providers = %q, %q", rows[0].Provider, rows[1].Provider)
	}
}

func TestFileStoreName(t *testing.T) {
	if DefaultPath("/tmp/agent") != filepath.Join("/tmp/agent", FileName) {
		t.Errorf("DefaultPath = %q", DefaultPath("/tmp/agent"))
	}
	if DefaultPath("") != "" {
		t.Errorf("DefaultPath of an empty directory = %q", DefaultPath(""))
	}
	if FileName != "models-metadata.json" {
		t.Errorf("FileName = %q", FileName)
	}
}
