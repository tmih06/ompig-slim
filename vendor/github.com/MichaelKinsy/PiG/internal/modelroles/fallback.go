package modelroles

import (
	"slices"
	"strings"
)

// Lookup is the exact catalog lookup needed to preserve literal @ IDs over route
// parsing and translate aggregator/vendor prefixes only when the catalog agrees.
type Lookup func(provider, id string) bool

// Selector is one canonical fallback candidate with independent effort/routing.
type Selector struct{ Raw, Provider, ID, Thinking, Upstream string }

// ParseSelector uses exact-first literal matching, then @upstream routing. Unknown
// models remain parseable so discovery can populate them later; malformed and role
// aliases are rejected here and resolved by Config.Resolve before parsing.
func ParseSelector(raw string, lookup Lookup) (Selector, bool) {
	base, thinking := Split(strings.TrimSpace(raw))
	provider, id, ok := strings.Cut(base, "/")
	if !ok || provider == "" || id == "" || strings.HasPrefix(raw, "@") || strings.ContainsAny(base, " \t\n") {
		return Selector{}, false
	}
	upstream := ""
	if lookup == nil || !lookup(provider, id) {
		if i := strings.LastIndex(id, "@"); i > 0 {
			upstream = id[i+1:]
			id = id[:i]
		}
	}
	selector := Selector{Provider: provider, ID: id, Thinking: thinking, Upstream: upstream}
	selector.Raw = selector.String()
	return selector, true
}

// String preserves canonical route-before-thinking order and omits inherit.
func (s Selector) String() string {
	raw := s.Provider + "/" + s.ID
	if s.Upstream != "" {
		raw += "@" + s.Upstream
	}
	if s.Thinking != "" && s.Thinking != "inherit" {
		raw += ":" + s.Thinking
	}
	return raw
}

// Base is the model-plus-route identity; efforts are intentionally excluded.
func (s Selector) Base() string { s.Thinking = ""; return s.String() }

// ExpandWildcard ports provider/prefix/*: it re-prefixes the failing model's bare
// ID; plain provider/* preserves IDs unless a direct-provider lookup requires the
// aggregator vendor prefix to be removed.
func ExpandWildcard(entry string, current Selector, lookup Lookup) string {
	if !strings.HasSuffix(entry, "/*") {
		return entry
	}
	provider, prefix, _ := strings.Cut(strings.TrimSuffix(entry, "/*"), "/")
	id := current.ID
	bare := id[strings.LastIndex(id, "/")+1:]
	if prefix != "" {
		id = prefix + "/" + bare
	} else if bare != id && lookup != nil && !lookup(provider, id) && lookup(provider, bare) {
		id = bare
	}
	return provider + "/" + id
}

// ResolveChainKey ports exact-effort, base-model, longest-wildcard, explicit role,
// matching role (default first), then default precedence. Kind roles participate
// only when explicitly hinted, preventing a shared web/judge model stealing chat.
func ResolveChainKey(config Config, chains map[string][]string, role, current string, lookup Lookup) string {
	parsed, ok := ParseSelector(current, lookup)
	if !ok {
		if _, exists := chains[role]; exists {
			return role
		}
		return ""
	}
	keys := mapKeys(chains)
	slices.Sort(keys)
	baseKey := ""
	for _, key := range keys {
		if strings.HasSuffix(key, "/*") || !strings.Contains(key, "/") {
			continue
		}
		candidate, valid := ParseSelector(key, lookup)
		if !valid {
			continue
		}
		if candidate.Raw == parsed.Raw {
			return key
		}
		if candidate.Base() == parsed.Base() && candidate.Thinking == "" {
			baseKey = key
		}
	}
	if baseKey != "" {
		return baseKey
	}
	best := ""
	for _, key := range keys {
		if !strings.HasSuffix(key, "/*") {
			continue
		}
		prefix := strings.TrimSuffix(key, "*")
		if strings.HasPrefix(parsed.Provider+"/"+parsed.ID, prefix) && len(key) > len(best) {
			best = key
		}
	}
	if best != "" {
		return best
	}
	if _, exists := chains[role]; role != "" && exists {
		return role
	}
	matched := ""
	for _, key := range keys {
		if strings.Contains(key, "/") || slices.Contains([]string{"web", "image", "speech", "dictation", "judge"}, key) {
			continue
		}
		value, err := config.Resolve("@"+key, "")
		if err != nil {
			continue
		}
		candidate, valid := ParseSelector(value, lookup)
		if valid && candidate.Base() == parsed.Base() {
			if key == "default" {
				return key
			}
			if matched == "" {
				matched = key
			}
		}
	}
	if matched != "" {
		return matched
	}
	if len(chains["default"]) > 0 {
		return "default"
	}
	return ""
}

// mapKeys yields deterministic resolution without leaking mutable map storage.
func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

// EffectiveCandidates resolves aliases and wildcard tails, deduplicates canonical
// selectors, and returns the tail after current. wrap adds earlier entries for
// credential/usage prewalking; ordinary retry never wraps.
func EffectiveCandidates(config Config, primary, current, legacy string, chain []string, lookup Lookup, wrap bool) []Selector {
	parsed, ok := ParseSelector(current, lookup)
	if !ok {
		return nil
	}
	entries := append([]string{primary}, chain...)
	all := []Selector{}
	seen := map[string]bool{}
	exact, base := -1, -1
	for _, entry := range entries {
		value := ExpandWildcard(entry, parsed, lookup)
		resolved, err := config.Resolve(value, legacy)
		if err != nil {
			continue
		}
		candidate, valid := ParseSelector(resolved, lookup)
		if !valid || seen[candidate.Raw] {
			continue
		}
		seen[candidate.Raw] = true
		all = append(all, candidate)
		if candidate.Raw == parsed.Raw {
			exact = len(all) - 1
		}
		if base < 0 && candidate.Base() == parsed.Base() {
			base = len(all) - 1
		}
	}
	index := exact
	if index < 0 {
		index = base
	}
	if index < 0 {
		return all
	}
	out := slices.Clone(all[index+1:])
	if wrap {
		out = append(out, all[:index]...)
	}
	return out
}
