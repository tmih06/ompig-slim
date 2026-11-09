package codingagent

import (
	"runtime"
	"slices"

	"github.com/MichaelKinsy/PiG/ai"
)

// BrowserProviderIDs is OMP's provider roster plus explicit native additions.
// Old bundled alias providers must not pollute locked-provider navigation; an
// authenticated alias or user/extension-defined provider remains selectable.
// Empty OMP providers are included so their login/discovery entry is not lost.
func (r *ModelRegistry) BrowserProviderIDs() []string {
	ids := ai.OMPCatalogProviders()
	if runtime.GOOS != "darwin" {
		ids = slices.DeleteFunc(ids, func(id string) bool { return id == "apple" })
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	r.mu.RLock()
	if r.config != nil {
		for id := range r.config.Providers {
			add(id)
		}
	}
	for id := range r.dynamic {
		add(id)
	}
	r.mu.RUnlock()
	for _, id := range r.modelProviderIDs() {
		if !seen[id] && r.HasConfiguredAuth(id) {
			add(id)
		}
	}
	slices.Sort(ids)
	return ids
}
