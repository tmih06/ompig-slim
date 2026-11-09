package modelmetadata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestDiscoverySnapshotRevalidatesWithoutLosingEligibility verifies the shared
// source's normal lifecycle: one fetch, no redundant fetch inside TTL, ETag 304
// after expiry, and restart with tool support/kind/limits intact on disk.
func TestDiscoverySnapshotRevalidatesWithoutLosingEligibility(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if calls.Load() > 1 {
			if r.Header.Get("If-None-Match") != `"discovery"` {
				t.Error("missing validator", r.Header)
			}
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", `"discovery"`)
		fmt.Fprint(w, `{"openai":{"models":{"future":{"tool_call":true,"kind":"chat","modalities":{"input":["text"]},"limit":{"context":128000,"output":4096}}}}}`)
	}))
	defer server.Close()
	clock := time.Now()
	store := NewFileStore(filepath.Join(t.TempDir(), FileName))
	service := New(Options{URL: server.URL, Store: store, Now: func() time.Time { return clock }})
	first, err := service.Refresh(context.Background(), RefreshOptions{})
	if err != nil || len(first.Rows) != 1 {
		t.Fatal(first, err)
	}
	generation := service.Generation()
	if _, err = service.Refresh(context.Background(), RefreshOptions{}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("fetched inside TTL", calls.Load())
	}
	clock = clock.Add(TTL + time.Second)
	checked, err := service.Refresh(context.Background(), RefreshOptions{})
	if err != nil || checked.Stale || checked.Error != "" || calls.Load() != 2 {
		t.Fatal(checked, err, calls.Load())
	}
	if service.Generation() != generation {
		t.Fatal("304 invalidated catalog projection")
	}
	restored := New(Options{Store: store})
	if err = restored.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows := restored.Rows()
	if len(rows) != 1 || rows[0].ToolCall == nil || !*rows[0].ToolCall || rows[0].Kind != "chat" || rows[0].ContextWindow == nil || *rows[0].ContextWindow != 128000 {
		t.Fatal("lost discovery eligibility on restart", rows)
	}
}

// TestOldMetricSnapshotForcesUnconditionalDiscoveryUpgrade preserves useful
// schema-1 offline scores while ensuring an otherwise-fresh ETag cannot cause
// perpetual 304s without the tool_call fields needed for discovery.
func TestOldMetricSnapshotForcesUnconditionalDiscoveryUpgrade(t *testing.T) {
	clock := time.Now()
	score := 42.0
	store := NewInMemoryStore()
	if err := store.Write(context.Background(), ai.ModelCatalogMetadataSnapshot{Version: 1, FetchedAt: clock.UnixMilli(), CheckedAt: clock.UnixMilli(), ETag: `"old"`, Rows: []ai.ModelCatalogMetadataRow{{Provider: "openai", ID: "old", Int: &score}}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" {
			t.Error("old schema sent conditional validator")
		}
		fmt.Fprint(w, `{"openai":{"models":{"future":{"tool_call":true,"modalities":{"input":["text"]},"limit":{"context":128000,"output":4096}}}}}`)
	}))
	defer server.Close()
	service := New(Options{Store: store, URL: server.URL, Now: func() time.Time { return clock }})
	if err := service.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(service.Rows()) != 1 || service.Rows()[0].Int == nil {
		t.Fatal("discarded usable old metrics offline")
	}
	result, err := service.Refresh(context.Background(), RefreshOptions{})
	if err != nil || len(result.Rows) != 1 || result.Rows[0].ToolCall == nil {
		t.Fatal("old schema did not upgrade", result, err)
	}
}
