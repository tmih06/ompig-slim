// Package modelroles owns OMP-compatible role selectors and ordered fallback resolution.
package modelroles

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Tag supplies display metadata for a built-in or user-defined role.
type Tag struct {
	Name                         string `json:"name"`
	Color                        string `json:"color,omitempty"`
	Hidden                       bool   `json:"hidden,omitempty"`
	nameSet, colorSet, hiddenSet bool
}

// Config is shared by settings and the model hub. Nil cycle order uses OMP's defaults;
// an explicit empty list disables role cycling. Callers own all maps and slices.
type Config struct {
	ModelRecent        []string          `json:"modelRecent,omitzero"`
	ModelProviderOrder []string          `json:"modelProviderOrder,omitzero"`
	DisabledProviders  []string          `json:"disabledProviders,omitzero"`
	ModelRoles         map[string]string `json:"modelRoles,omitempty"`
	ModelTags          map[string]Tag    `json:"modelTags,omitempty"`
	CycleOrder         []string          `json:"cycleOrder,omitzero"`
	ModelRoleStorage   string            `json:"modelRoleStorage,omitempty"`
}

var roleNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// ValidName accepts custom roles starting with a letter followed by letters,
// digits, underscores, or hyphens; invalid names never become assignment keys.
func ValidName(name string) bool { return roleNamePattern.MatchString(name) }

// Builtins returns OMP's stable role order, without sharing mutable storage.
func Builtins() []string {
	return []string{"default", "smol", "slow", "vision", "plan", "commit", "tiny", "memory", "task", "advisor", "widget", "image", "web", "speech", "dictation", "judge"}
}

// Clone detaches settings snapshots from their writer.
func (c Config) Clone() Config {
	c.ModelRoles = maps.Clone(c.ModelRoles)
	c.ModelRecent = slices.Clone(c.ModelRecent)
	c.ModelProviderOrder = slices.Clone(c.ModelProviderOrder)
	c.DisabledProviders = slices.Clone(c.DisabledProviders)
	c.ModelTags = maps.Clone(c.ModelTags)
	c.CycleOrder = slices.Clone(c.CycleOrder)
	return c
}

// Merge overlays only authored entries; project roles do not erase unrelated globals.
func (c Config) Merge(p Config) Config {
	c = c.Clone()
	if c.ModelRoles == nil {
		c.ModelRoles = map[string]string{}
	}
	maps.Copy(c.ModelRoles, p.ModelRoles)
	if p.ModelRecent != nil {
		c.ModelRecent = slices.Clone(p.ModelRecent)
	}
	if p.ModelProviderOrder != nil {
		c.ModelProviderOrder = slices.Clone(p.ModelProviderOrder)
	}
	if p.DisabledProviders != nil {
		c.DisabledProviders = slices.Clone(p.DisabledProviders)
	}
	if c.ModelTags == nil {
		c.ModelTags = map[string]Tag{}
	}
	for role, override := range p.ModelTags {
		tag := c.ModelTags[role]
		if override.Name != "" || override.nameSet {
			tag.Name = override.Name
			tag.nameSet = override.nameSet
		}
		if override.Color != "" || override.colorSet {
			tag.Color = override.Color
			tag.colorSet = override.colorSet
		}
		if override.Hidden || override.hiddenSet {
			tag.Hidden = override.Hidden
			tag.hiddenSet = override.hiddenSet
		}
		c.ModelTags[role] = tag
	}
	if p.CycleOrder != nil {
		c.CycleOrder = slices.Clone(p.CycleOrder)
	}
	if p.ModelRoleStorage != "" {
		c.ModelRoleStorage = p.ModelRoleStorage
	}
	return c
}

// Order returns the configured quick-switch order, or OMP's built-in order.
func (c Config) Order() []string {
	if c.CycleOrder != nil {
		return slices.Clone(c.CycleOrder)
	}
	return []string{"smol", "default", "slow"}
}

// Known returns built-ins first, followed by deterministic custom roles from all settings.
func (c Config) Known(chains map[string][]string) []string {
	roles := Builtins()
	extra := map[string]bool{}
	for role := range c.ModelRoles {
		extra[role] = true
	}
	for role := range c.ModelTags {
		extra[role] = true
	}
	for _, role := range c.Order() {
		extra[role] = true
	}
	for role := range chains {
		if !strings.Contains(role, "/") {
			extra[role] = true
		}
	}
	for _, role := range slices.Sorted(maps.Keys(extra)) {
		if !slices.Contains(roles, role) {
			roles = append(roles, role)
		}
	}
	return roles
}

// Role recognizes canonical @role, legacy pi/role, and the default shorthand *.
func Role(selector string) (string, bool) {
	if selector == "*" {
		return "default", true
	}
	for _, prefix := range []string{"@", "pi/"} {
		if strings.HasPrefix(selector, prefix) {
			return strings.TrimPrefix(selector, prefix), true
		}
	}
	return "", false
}

// Split removes only a recognized thinking suffix, preserving routing variants and IDs.
func Split(selector string) (string, string) {
	i := strings.LastIndex(selector, ":")
	if i < 0 {
		return selector, ""
	}
	switch selector[i+1:] {
	case "off", "minimal", "low", "medium", "high", "xhigh", "max", "auto", "inherit":
		return selector[:i], selector[i+1:]
	}
	return selector, ""
}

// Resolve follows role aliases and inheritance, rejecting cycles and unassigned roles.
// legacyDefault is the provider-qualified pre-role default, used only for default.
func (c Config) Resolve(selector, legacyDefault string) (string, error) {
	seen := map[string]bool{}
	effort := ""
	for {
		base, level := Split(selector)
		if effort == "" {
			effort = level
		}
		selector = base
		role, ok := Role(selector)
		if !ok {
			if selector == "" || selector == "/" {
				return "", fmt.Errorf("no model assigned")
			}
			if effort != "" {
				selector += ":" + effort
			}
			return selector, nil
		}
		if seen[role] {
			return "", fmt.Errorf("model role cycle at @%s", role)
		}
		seen[role] = true
		value := c.ModelRoles[role]
		if value == "" {
			switch role {
			case "default":
				value = legacyDefault
			case "slow", "plan", "task", "vision", "commit":
				value = "@default"
			case "tiny", "widget":
				value = "@smol"
			case "memory":
				value = "@tiny"
			case "advisor":
				value = "@slow"
			case "smol":
				value = "@default"
			}
		}
		if value == "" {
			return "", fmt.Errorf("model role @%s is unassigned", role)
		}
		selector = value
	}
}

// CloneChains preserves explicit empty chains: empty disables inheritance, unlike absence.
func CloneChains(chains map[string][]string) map[string][]string {
	if chains == nil {
		return nil
	}
	out := make(map[string][]string, len(chains))
	for k, v := range chains {
		out[k] = slices.Clone(v)
	}
	return out
}

// ChainKey chooses exact model, longest provider wildcard, then hinted role, as
// OMP does. An explicit empty role chain suppresses default-chain inheritance.
func ChainKey(chains map[string][]string, role, model string) string {
	if _, ok := chains[model]; ok {
		return model
	}
	best := ""
	for key := range chains {
		if strings.HasSuffix(key, "/*") && strings.HasPrefix(model, strings.TrimSuffix(key, "*")) && len(key) > len(best) {
			best = key
		}
	}
	if best != "" {
		return best
	}
	if _, ok := chains[role]; role != "" && ok {
		return role
	}
	if len(chains["default"]) > 0 {
		return "default"
	}
	return ""
}

// Candidates returns the tail after current (never wraps), deduplicated in configured order.
// Wildcard entries retain the current model ID when moving between providers.
func Candidates(primary, current string, chain []string) []string {
	all := []string{primary}
	all = append(all, chain...)
	seen := map[string]bool{}
	out := []string{}
	currentBase, _ := Split(current)
	start := -1
	for _, entry := range all {
		if strings.HasSuffix(entry, "/*") {
			_, id, _ := strings.Cut(currentBase, "/")
			entry = strings.TrimSuffix(entry, "*") + id
		}
		if entry == "" || seen[entry] {
			continue
		}
		seen[entry] = true
		out = append(out, entry)
		base, _ := Split(entry)
		if entry == current || (start < 0 && base == currentBase) {
			start = len(out) - 1
		}
	}
	if start >= 0 {
		return out[start+1:]
	}
	return out
}
