package codingagent

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// TestModelPresetsAtomicLayersAndPersistence covers same-name atomic replacement,
// null tombstones, active detection, capture ownership, and real file round trips.
func TestModelPresetsAtomicLayersAndPersistence(t *testing.T) {
	global := Settings{Config: modelroles.Config{ModelRoles: map[string]string{"default": "p/a", "review": "p/b"}}, DefaultThinkingLevel: "high"}
	sm := NewInMemorySettingsManager(global)
	if err := sm.SaveModelPreset("work"); err != nil {
		t.Fatal(err)
	}
	if active := sm.Get().Config.ActivePreset("high"); active != "work" {
		t.Fatal(active)
	}
	project := json.RawMessage(`{"modelRoles":{"default":"q/c"}}`)
	if err := sm.UpdateProject(func(s *Settings) { s.ModelPresets = map[string]json.RawMessage{"work": project} }); err != nil {
		t.Fatal(err)
	}
	preset, err := sm.Get().Config.Preset("work")
	if err != nil || len(preset.ModelRoles) != 1 || preset.ModelRoles["default"] != "q/c" || preset.DefaultThinkingLevel != "" {
		t.Fatal("same-name preset merged", preset, err)
	}
	if result, err := sm.DeleteModelPreset("work"); err != nil || result != "project" {
		t.Fatal(result, err)
	}
	if err := sm.UpdateProject(func(s *Settings) { s.ModelPresets["work"] = json.RawMessage(`null`) }); err != nil {
		t.Fatal(err)
	}
	if names := sm.Get().Config.PresetNames(); len(names) != 0 {
		t.Fatal("tombstone listed", names)
	}
	file := NewSettingsManager(t.TempDir(), t.TempDir())
	if err := file.SetModelRole("default", "p/a:off", "global"); err != nil {
		t.Fatal(err)
	}
	if err := file.SaveModelPreset("work"); err != nil {
		t.Fatal(err)
	}
	reloaded := NewSettingsManager(file.cwd, file.agentDir)
	preset, err = reloaded.Get().Config.Preset("work")
	if err != nil || preset.ModelRoles["default"] != "p/a:off" {
		t.Fatal(preset, err)
	}
	for _, name := range []string{"", "1bad", "bad name", "a/b"} {
		if err := file.SaveModelPreset(name); err == nil {
			t.Fatal("invalid name accepted", name)
		}
	}
}

// TestModelPresetWritesScopeAndReportsShadowing keeps project assignments intact.
func TestModelPresetWritesScopeAndReportsShadowing(t *testing.T) {
	sm := NewInMemorySettingsManager(Settings{Config: modelroles.Config{ModelRoles: map[string]string{"default": "p/a", "stale": "p/b"}}})
	if err := sm.SetModelRole("default", "q/a", "project"); err != nil {
		t.Fatal(err)
	}
	shadowed, err := sm.WriteModelPreset(modelroles.Preset{ModelRoles: map[string]string{"default": "p/c"}, DefaultThinkingLevel: "off"})
	if err != nil || !reflect.DeepEqual(shadowed, []string{"default"}) {
		t.Fatal(shadowed, err)
	}
	if _, ok := sm.GetGlobalSettings().ModelRoles["stale"]; ok {
		t.Fatal("preset retained absent role")
	}
	if sm.Get().ModelRoles["default"] != "q/a" {
		t.Fatal("project clobbered")
	}
}

// TestModelMRUIsBoundedAndDetached never copies project-only model usage globally.
func TestModelMRUIsBoundedAndDetached(t *testing.T) {
	sm := NewInMemorySettingsManager(Settings{})
	for _, selector := range []string{"p/a", "p/b", "p/a"} {
		if err := sm.RecordModelUsage(selector); err != nil {
			t.Fatal(err)
		}
	}
	if got := sm.Get().ModelRecent; !reflect.DeepEqual(got, []string{"p/a", "p/b"}) {
		t.Fatal(got)
	}
	copy := sm.Get()
	copy.ModelRecent[0] = "changed"
	if sm.Get().ModelRecent[0] != "p/a" {
		t.Fatal("MRU shares mutable settings")
	}
}
