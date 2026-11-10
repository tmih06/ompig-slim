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

// RoleRef reports the role an entry points at, if it points at one at all. Only
// an explicit role spelling counts; a bare model fragment that happens to share a
// role's name must never be expanded as that role.
func RoleRef(entry string) (string, bool) {
	base, _ := Split(strings.TrimSpace(entry))
	return Role(base)
}

// ChainEntries flattens key's chain, expanding each role entry depth-first through
// the referenced role's own chain. A role destination therefore means that role's
// model first and then that role's own recovery plan, in the order the author
// wrote it. Every role is entered at most once, so a reference loop — written by
// hand in one settings file, or closed across the global and project layers —
// degrades to a shorter chain instead of expanding forever. The result is raw
// entries: alias resolution and deduplication stay in EffectiveCandidates so both
// paths agree on canonical identity.
func ChainEntries(chains map[string][]string, key string) []string {
	entries := []string{}
	visited := map[string]bool{}
	var expand func(string)
	expand = func(node string) {
		if visited[node] {
			return
		}
		visited[node] = true
		for _, entry := range chains[node] {
			entries = append(entries, entry)
			if role, ok := RoleRef(entry); ok {
				expand(role)
			}
		}
	}
	expand(key)
	return entries
}

// ChainReaches reports whether from's chain reaches target by following role
// entries. A role reaches itself, so from == target reports true, which is what
// makes a self reference a loop.
//
// Appending @from to target's chain closes a loop exactly when this reports true,
// because every other edge on that loop already exists: the walk covers the saved
// chains, so the write guard needs no prospective copy. It walks the same edges as
// ChainEntries, and both terminate on a loop that is already saved.
func ChainReaches(chains map[string][]string, from, target string) bool {
	visited := map[string]bool{}
	var walk func(string) bool
	walk = func(role string) bool {
		if role == target {
			return true
		}
		// A stored loop only revisits edges already known not to reach target.
		if visited[role] {
			return false
		}
		visited[role] = true
		for _, entry := range chains[role] {
			if ref, ok := RoleRef(entry); ok && walk(ref) {
				return true
			}
		}
		return false
	}
	return walk(from)
}

// AliasReaches reports whether following from's assignment reaches target. A role
// reaches itself, so from == target reports true, which is what makes a self
// assignment a loop.
//
// Only ModelRoles aliases and the built-in inheritance Resolve applies are edges,
// so this answers "would assigning @target to from cycle?" — a different graph from
// a chain's role references, which ChainReaches walks. A value naming a model and
// an unassigned role both end the walk, because Resolve stops there too.
func AliasReaches(config Config, from, target string) bool {
	seen := map[string]bool{}
	for role := from; role != ""; {
		if role == target {
			return true
		}
		if seen[role] {
			return false
		}
		seen[role] = true
		role, _ = RoleRef(config.assignment(role, ""))
	}
	return false
}

// FallbackRoles returns the roles key's chain may reference, in known-role order:
// exactly those whose entry the write guard accepts, because a role that already
// reaches key would close a reference loop. Kind compatibility is deliberately not
// decided here — the catalog's kind families live in the TUI — so the hub applies
// its own kind rule on top.
func FallbackRoles(config Config, chains map[string][]string, key string) []string {
	return filterRoles(config.Known(chains), func(role string) bool { return !ChainReaches(chains, role, key) })
}

// AssignableRoles returns the roles from's primary assignment may alias, in
// known-role order: exactly those that cannot reach from, so every offered choice
// resolves instead of producing a cycle.
//
// Assignment is a different graph from a chain's role references — it follows
// ModelRoles aliases, not chains — so this is not ChainReaches with another name.
func AssignableRoles(config Config, chains map[string][]string, from string) []string {
	return filterRoles(config.Known(chains), func(role string) bool { return !AliasReaches(config, role, from) })
}

// filterRoles keeps the roles keep accepts, preserving order and never returning
// nil so callers can range over the result unconditionally.
func filterRoles(roles []string, keep func(string) bool) []string {
	out := []string{}
	for _, role := range roles {
		if keep(role) {
			out = append(out, role)
		}
	}
	return out
}

// ModelRoleEffort returns the effort suffix of a saved role assignment or chain
// entry, or "" when it carries none. "inherit" means "use whatever the resolved
// model defaults to" and is reported as empty so callers can treat it as absent.
// A role entry's suffix belongs to the alias, so an effort edit rewrites the alias
// instead of pinning the model it currently resolves to.
func ModelRoleEffort(entry string) string {
	_, level := Split(strings.TrimSpace(entry))
	if level == "inherit" {
		return ""
	}
	return level
}

// EffectiveCandidates resolves aliases, wildcard tails, and role references,
// deduplicates canonical selectors, and returns the tail after current. chain
// must already be flattened, i.e. the output of ChainEntries for its key. wrap
// adds earlier entries for credential/usage prewalking; ordinary retry never wraps.
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
