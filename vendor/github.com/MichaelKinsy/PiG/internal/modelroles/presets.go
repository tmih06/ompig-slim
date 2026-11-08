package modelroles

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Preset captures the same atomic role/thinking snapshot as OMP. Fallback chains,
// cycle order, and credentials are deliberately not part of a model preset.
type Preset struct {
	ModelRoles           map[string]string `json:"modelRoles"`
	DefaultThinkingLevel string            `json:"defaultThinkingLevel,omitempty"`
}

var presetNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// ValidName is shared by custom-role creation and preset commands.
func ValidName(name string) bool { return presetNamePattern.MatchString(name) }

// PresetNames lists visible names in lexical order, excluding null tombstones.
func (c Config) PresetNames() []string {
	var names []string
	for name, raw := range c.ModelPresets {
		if strings.TrimSpace(string(raw)) != "null" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// Preset reads and validates one whole highest-layer entry. An invalid preset
// returns a diagnostic instead of poisoning the remainder of the settings file.
func (c Config) Preset(name string) (Preset, error) {
	raw, ok := c.ModelPresets[name]
	if !ok || strings.TrimSpace(string(raw)) == "null" {
		return Preset{}, fmt.Errorf("Preset not found: %s", name)
	}
	var p Preset
	if err := json.Unmarshal(raw, &p); err != nil || p.ModelRoles == nil {
		return Preset{}, fmt.Errorf("Preset %q is malformed: modelRoles must be a mapping", name)
	}
	for role, value := range p.ModelRoles {
		if strings.TrimSpace(value) == "" {
			return Preset{}, fmt.Errorf("Preset %q is malformed: role %q is not a model selector", name, role)
		}
	}
	if p.DefaultThinkingLevel != "" {
		base, level := Split("m:" + p.DefaultThinkingLevel)
		if base != "m" || level == "" || level == "inherit" {
			return Preset{}, fmt.Errorf("Preset %q is malformed: defaultThinkingLevel is not a thinking level", name)
		}
	}
	return p, nil
}

// CapturePreset detaches nonempty effective assignments and the configured effort.
func (c Config) CapturePreset(thinking string) Preset {
	p := Preset{ModelRoles: maps.Clone(c.ModelRoles), DefaultThinkingLevel: thinking}
	if p.ModelRoles == nil {
		p.ModelRoles = map[string]string{}
	}
	for role, value := range p.ModelRoles {
		if value == "" {
			delete(p.ModelRoles, role)
		}
	}
	return p
}

// ActivePreset returns the first exact role/effort match, ignoring empty selectors.
func (c Config) ActivePreset(thinking string) string {
	current := c.CapturePreset(thinking)
	for _, name := range c.PresetNames() {
		p, err := c.Preset(name)
		if err == nil && maps.Equal(p.ModelRoles, current.ModelRoles) && (p.DefaultThinkingLevel == "" || p.DefaultThinkingLevel == thinking) {
			return name
		}
	}
	return ""
}
