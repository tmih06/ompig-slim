package tui

import "github.com/MichaelKinsy/PiG/ai"

// ModelSelectorItem is one row in the picker.
type ModelSelectorItem struct {
	// Kind is the OMP catalog kind; empty means chat. Non-chat rows carry
	// descriptor-only metadata in Model and are never sent through chat APIs.
	Kind string
	// Role preserves logical identity in the quick picker; empty is a direct catalog selection.
	Role     string
	Provider string
	ID       string // bare model id (e.g. "gpt-4o")
	// Name is the raw model display name (upstream model.name), empty when
	// the source has none. Search text omits an empty name; the selected-model
	// footer falls back to ID.
	Name string
	// Model is the enriched model the row came from, when the caller has one; it supplies the
	// catalog metric columns and the selected-model fact block. A host-driven picker that only
	// knows ids and names leaves it nil and the row shows identity alone.
	Model *ai.Model
}

// FQ returns the "<provider>/<id>" form used for switch dispatch.
func (m ModelSelectorItem) FQ() string {
	if m.Role != "" {
		return "@" + m.Role
	}
	return m.Provider + "/" + m.ID
}
