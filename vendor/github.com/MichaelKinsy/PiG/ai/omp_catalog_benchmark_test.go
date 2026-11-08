package ai

import (
	"runtime"
	"testing"
)

// BenchmarkOMPCatalogLookup measures a warmed exact selector, not gzip startup.
// It exercises the shared loader used by roles, metrics and every catalog read.
func BenchmarkOMPCatalogLookup(b *testing.B) {
	_, _ = LookupOMPChatModel("openai", "gpt-6.1-sol")
	var model GeneratedModel
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model, _ = LookupOMPChatModel("openai", "gpt-6.1-sol")
	}
	runtime.KeepAlive(model)
}

// BenchmarkOMPCatalogToModel includes request-data cloning and display metric
// projection. The warmed seed remains immutable across all benchmark iterations.
func BenchmarkOMPCatalogToModel(b *testing.B) {
	row, _ := LookupOMPChatModel("openai", "gpt-6.1-sol")
	_ = row.ToModel()
	var model *Model
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model = row.ToModel()
	}
	runtime.KeepAlive(model)
}
