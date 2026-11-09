package codingagent

import (
	"runtime"
	"testing"
)

// BenchmarkModelCatalog measures warmed startup-style identity resolution and
// full typed catalog composition over every bundled provider. Separate cases
// expose whether lookup improvements survive real registry work. All settings
// are isolated and offline; no credential or provider endpoint is contacted.
func BenchmarkModelCatalog(b *testing.B) {
	b.Setenv("PI_OFFLINE", "1")
	registry := NewModelRegistry(b.TempDir())
	defer registry.CloseModelTasks()
	_ = registry.RuntimeModels()
	_ = registry.GetAllProviderModelData()
	b.Run("RuntimeModels", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			models := registry.RuntimeModels()
			runtime.KeepAlive(models)
		}
	})
	b.Run("AllModels", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			models := registry.GetAllProviderModelData()
			runtime.KeepAlive(models)
		}
	})
	b.Run("ExactModel", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			model := registry.GetProviderModelDataByID("openai", "gpt-6.1-sol")
			runtime.KeepAlive(model)
		}
	})
}
