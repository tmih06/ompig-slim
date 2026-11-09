package codingagent

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestObsoleteModelSettingsAreInert prevents retired settings from filtering the
// catalog or creating migration roles. Unknown settings retain the normal
// lossless JSON behavior, but have no typed field, schema entry or live setter.
func TestObsoleteModelSettingsAreInert(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("OPENAI_API_KEY", "offline-fixture")
	var settings Settings
	if err := json.Unmarshal([]byte(`{"enabledModels":["openai/not-a-model"],"modelRoles":{"default":"openai/gpt-4o","smol":"openai/gpt-4o-mini"},"cycleOrder":["default","smol"]}`), &settings); err != nil {
		t.Fatal(err)
	}
	sm := NewInMemorySettingsManager(settings)
	registry := NewModelRegistry(t.TempDir())
	defer registry.CloseModelTasks()
	mode := &InteractiveMode{opts: InteractiveModeOptions{SettingsManager: sm, ModelRegistry: registry}}
	count := 0
	for _, item := range mode.availableModelItems() {
		if item.Provider == "openai" {
			count++
		}
	}
	if count < 2 {
		t.Fatal("obsolete selection still filters the runtime catalog", count)
	}
	if got := sm.Get().ModelRoles; !reflect.DeepEqual(got, settings.ModelRoles) {
		t.Fatal("obsolete selection created or changed roles", got)
	}
	if got := sm.ModelRoleCycle(); !reflect.DeepEqual(got, []string{"default", "smol"}) {
		t.Fatal("obsolete selection changed cycling", got)
	}
	if _, exists := reflect.TypeFor[Settings]().FieldByName("EnabledModels"); exists {
		t.Fatal("obsolete typed setting survives")
	}
}
