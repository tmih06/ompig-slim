package modelroles

import (
	"fmt"
	"strings"
)

// Candidate is the catalog identity/role-kind surface needed for automatic choice.
// Auth filtering is the host's responsibility; catalog matching never performs I/O.
type Candidate struct{ Provider, ID, Kind string }

// ResolveAvailable prefers configured aliases, then OMP's built-in priority chain.
// Explicit provider-qualified values must match exactly; bare fragments retain
// fuzzy substring matching. Configured defaults precede smol/slow priority only.
func (c Config) ResolveAvailable(selector, legacy string, pool []Candidate) (string, error) {
	return c.resolveAvailable(selector, legacy, pool, map[string]bool{})
}

// resolveAvailable tracks the alias stack independently per priority alternative,
// so a failed candidate cannot poison an unrelated fallback role.
func (c Config) resolveAvailable(selector, legacy string, pool []Candidate, seen map[string]bool) (string, error) {
	base, effort := Split(selector)
	role, isRole := Role(base)
	patterns := []string{base}
	if isRole {
		if seen[role] {
			return "", fmt.Errorf("model role cycle at @%s", role)
		}
		next := map[string]bool{}
		for k, v := range seen {
			next[k] = v
		}
		next[role] = true
		seen = next
		if configured := c.ModelRoles[role]; configured != "" {
			patterns = []string{configured}
		} else {
			patterns = nil
			switch role {
			case "default":
				if legacy != "" && legacy != "/" {
					patterns = append(patterns, legacy)
				}
			case "smol", "slow":
				if value := c.ModelRoles["default"]; value != "" {
					patterns = append(patterns, value)
				}
			case "tiny", "widget":
				patterns = append(patterns, "@smol")
			case "memory":
				patterns = append(patterns, "@tiny")
			case "advisor":
				if c.ModelRoles["slow"] != "" {
					patterns = append(patterns, "@slow")
				}
			case "plan", "vision", "task", "commit":
				patterns = append(patterns, "@default")
			}
			patterns = append(patterns, RoleDefaults(role)...)
		}
	}
	for _, pattern := range patterns {
		value, level := Split(pattern)
		if effort != "" {
			level = effort
		}
		if _, nested := Role(value); nested {
			if level != "" {
				value += ":" + level
			}
			resolved, err := c.resolveAvailable(value, legacy, pool, seen)
			if err == nil {
				return resolved, nil
			}
			continue
		}
		matches := []Candidate{}
		route := ""
		literal := false
		for _, candidate := range pool {
			if strings.EqualFold(candidate.Provider+"/"+candidate.ID, value) || strings.EqualFold(candidate.ID, value) {
				literal = true
				break
			}
		}
		if !literal {
			if index := strings.LastIndex(value, "@"); index > strings.Index(value, "/") {
				route = value[index:]
				value = value[:index]
			}
		}
		qualified := strings.Contains(value, "/")
		for _, candidate := range pool {
			kind := candidate.Kind
			if kind == "" {
				kind = "chat"
			}
			if isRole && !acceptsKind(role, kind) {
				continue
			}
			full := candidate.Provider + "/" + candidate.ID
			if strings.EqualFold(full, value) || !qualified && strings.EqualFold(candidate.ID, value) {
				result := full + route
				if level != "" && level != "inherit" {
					result += ":" + level
				}
				return result, nil
			}
			if !qualified && strings.Contains(strings.ToLower(candidate.ID), strings.ToLower(value)) {
				matches = append(matches, candidate)
			}
		}
		if len(matches) > 0 {
			result := matches[0].Provider + "/" + matches[0].ID + route
			if level != "" && level != "inherit" {
				result += ":" + level
			}
			return result, nil
		}
	}
	if isRole && role == "default" && c.ModelRoles["default"] == "" {
		for _, candidate := range pool {
			if candidate.Kind == "" || candidate.Kind == "chat" {
				value := candidate.Provider + "/" + candidate.ID
				if effort != "" {
					value += ":" + effort
				}
				return value, nil
			}
		}
	}
	return "", fmt.Errorf("model selector %q does not resolve to an available model", selector)
}

// acceptsKind centralizes built-in role compatibility without chat fallback for
// image/speech/search-only roles.
func acceptsKind(role, kind string) bool {
	switch role {
	case "image":
		return kind == "image"
	case "speech":
		return kind == "tts"
	case "dictation":
		return kind == "stt"
	case "web":
		return kind == "search"
	case "tiny", "memory":
		return kind == "tiny" || kind == "classifier" || kind == "chat"
	case "judge":
		return kind == "judge" || kind == "tiny" || kind == "classifier" || kind == "chat"
	default:
		return kind == "chat"
	}
}
