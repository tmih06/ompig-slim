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

// ChainReaches reports the role-reference path that takes from's chain to target,
// as role names ending in target ("slow", "review" when @slow reaches @review),
// or nil when it does not reach it. A role reaches itself, so the result for
// from == target is that single name. from must be a role name: a model or
// provider key is a terminal chain, because only a role entry expands into
// another chain, so such a key never reaches anything and always reports nil.
//
// Appending @from to target's chain closes a loop exactly when this returns
// non-nil, because every other edge on that loop already exists: reachability
// covers the pre-existing chains, so callers do not need a prospective copy.
func ChainReaches(chains map[string][]string, from, target string) []string {
	if strings.Contains(from, "/") {
		return nil
	}
	if from == target {
		return []string{target}
	}
	path := []string{from}
	onPath := map[string]bool{from: true}
	var walk func(string) []string
	walk = func(role string) []string {
		for _, entry := range chains[role] {
			ref, ok := RoleRef(entry)
			if !ok {
				continue
			}
			if ref == target {
				return append(slices.Clone(path), target)
			}
			// Re-entering a role only revisits edges already explored, so a stored
			// loop terminates here rather than being re-walked.
			if onPath[ref] {
				continue
			}
			onPath[ref] = true
			path = append(path, ref)
			if found := walk(ref); found != nil {
				return found
			}
			path = path[:len(path)-1]
			delete(onPath, ref)
		}
		return nil
	}
	return walk(from)
}

// FallbackRoles returns the roles key's chain may reference, in known-role order.
// A role qualifies when appending it cannot close a reference loop, which is
// exactly what the write guard enforces, so every role the hub offers is one the
// host accepts. Kind compatibility is deliberately not decided here: the catalog's
// kind families live in the TUI, and asking this package to guess them would give
// two rules for one question.
func FallbackRoles(config Config, chains map[string][]string, key string) []string {
	roles := []string{}
	for _, role := range config.Known(chains) {
		if ChainReaches(chains, role, key) == nil {
			roles = append(roles, role)
		}
	}
	return roles
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

// AliasReaches reports the alias path taking from's assignment to target, as role
// names ending in target, or nil when it does not reach it. from reaches itself, so
// from == target yields that single name. Only ModelRoles aliases are followed:
// this answers "would assigning @target to from cycle?", which is a different graph
// from a chain's role references and must not be confused with ChainReaches.
// Assignment values that name a model end the walk, and an unassigned role ends it
// too, because Resolve stops at the first non-alias value.
func AliasReaches(config Config, from, target string) []string {
	if from == target {
		return []string{target}
	}
	path := []string{from}
	seen := map[string]bool{from: true}
	role := from
	for {
		value := config.ModelRoles[role]
		if value == "" {
			// Built-in inheritance is the one edge Resolve adds that settings do not
			// spell out, so follow the same fallbacks it uses.
			switch role {
			case "slow", "plan", "task", "vision", "commit", "smol":
				value = "@default"
			case "tiny", "widget":
				value = "@smol"
			case "memory":
				value = "@tiny"
			case "advisor":
				value = "@slow"
			}
		}
		next, ok := Role(strings.TrimSpace(value))
		if !ok {
			return nil
		}
		if next == target {
			return append(slices.Clone(path), target)
		}
		// A saved cycle re-enters a role whose edges are already walked.
		if seen[next] {
			return nil
		}
		seen[next] = true
		path = append(path, next)
		role = next
	}
}

// AssignableRoles returns the roles from's assignment may alias, in known-role
// order: exactly those that cannot reach from, so every offered choice resolves
// instead of producing a cycle. roles is the caller's display list, which keeps
// this independent of which names happen to be assigned in a settings layer.
func AssignableRoles(config Config, roles []string, from string) []string {
	out := []string{}
	for _, role := range roles {
		if AliasReaches(config, role, from) == nil {
			out = append(out, role)
		}
	}
	return out
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
