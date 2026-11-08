package tui

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/MichaelKinsy/PiG/internal/jsstring/localecompare"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

var browserVersionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:^|[-_])([0-9]+\.[0-9]+)`),
	regexp.MustCompile(`(?:^|[-_])([0-9]{1,2})-([0-9]{1,2})(?:-|$)`),
	regexp.MustCompile(`(?:^|[-_])([0-9]+)`),
}
var browserDatePattern = regexp.MustCompile(`-([0-9]{8})$`)

// browserVersion follows OMP's dot, short-dash, then integer precedence.
// Preserve the source regex behavior even for revision/date tails: changing
// which numeric segment matches changes the user-visible catalog order.
func browserVersion(id string) float64 {
	for _, pattern := range browserVersionPatterns {
		if match := pattern.FindStringSubmatch(id); match != nil {
			value := match[1]
			if len(match) > 2 {
				value += "." + match[2]
			}
			version, _ := strconv.ParseFloat(value, 64)
			return version
		}
	}
	return 0
}

// browserRankedItem owns sort keys for one ranking pass, not a persistent cache.
// Resolve role/MRU membership and parse IDs once per row rather than on every
// comparison. Rebuilding on query/data changes keeps live priorities, roles and
// MRU authoritative without invalidation state or an unbounded model-ID cache.
type browserRankedItem struct {
	item                 ModelSelectorItem
	fq                   string
	roleRank, recentRank int
	priority, version    float64
	latest               bool
	date                 string
}

// compareBrowserRankedItems is OMP modelItemComparator with precomputed keys:
// role rank, MRU, provider, priority, version descending, latest/date, locale ID.
// Role rows retain stable input order; search callers suppress only role rank.
func compareBrowserRankedItems(a, c browserRankedItem) int {
	if a.item.Role != "" || c.item.Role != "" {
		return 0
	}
	if a.roleRank != c.roleRank {
		return a.roleRank - c.roleRank
	}
	if a.recentRank != c.recentRank {
		if a.recentRank < c.recentRank {
			return -1
		}
		return 1
	}
	if a.item.Provider != c.item.Provider {
		if cmp := localecompare.Compare(a.item.Provider, c.item.Provider); cmp != 0 {
			return cmp
		}
	}
	if a.priority != c.priority {
		if a.priority < c.priority {
			return -1
		}
		return 1
	}
	if a.version != c.version {
		if a.version > c.version {
			return -1
		}
		return 1
	}
	if ar, cr := a.latest || a.date != "", c.latest || c.date != ""; ar != cr {
		if ar {
			return -1
		}
		return 1
	}
	if a.latest != c.latest {
		if a.latest {
			return -1
		}
		return 1
	}
	if a.date != "" && c.date != "" && a.date != c.date {
		return strings.Compare(c.date, a.date)
	}
	return localecompare.Compare(a.item.ID, c.item.ID)
}

// rankBrowserItems seats a separator only above genuinely non-role/non-MRU
// catalog entries. Current selection is not a separate ranking rule in OMP.
// All work and scratch ownership remain on the TUI owner loop.
func (b *ModelBrowser) rankBrowserItems() {
	b.items = slices.DeleteFunc(b.items, func(item ModelSelectorItem) bool { return item.Kind == "separator" })
	if len(b.items) == 0 || b.items[0].Role != "" {
		return
	}
	search := strings.TrimSpace(b.Query()) != ""
	roles := modelroles.Builtins()
	roleRank := make(map[string]int, len(roles))
	if !search {
		for i, role := range roles {
			base, _ := modelroles.Split(b.roles[role])
			if _, exists := roleRank[base]; !exists {
				roleRank[base] = i
			}
		}
	}
	recentRank := make(map[string]int, len(b.recent))
	for i, selector := range b.recent {
		if _, exists := recentRank[selector]; !exists {
			recentRank[selector] = i
		}
	}
	ranked := make([]browserRankedItem, len(b.items))
	for i, item := range b.items {
		key := browserRankedItem{item: item, fq: item.FQ(), roleRank: len(roles), recentRank: math.MaxInt, priority: math.MaxFloat64, version: browserVersion(item.ID), latest: strings.HasSuffix(item.ID, "-latest")}
		if rank, ok := roleRank[key.fq]; ok {
			key.roleRank = rank
		}
		if rank, ok := recentRank[key.fq]; ok {
			key.recentRank = rank
		}
		if item.Model != nil && item.Model.Priority != nil {
			key.priority = *item.Model.Priority
		}
		if match := browserDatePattern.FindStringSubmatch(item.ID); match != nil {
			key.date = match[1]
		}
		ranked[i] = key
	}
	slices.SortStableFunc(ranked, compareBrowserRankedItems)
	for i, key := range ranked {
		b.items[i] = key.item
	}
	if search {
		return
	}
	// Custom role assignments are preferred for separator placement, but do not
	// acquire a built-in role's sorting rank. Keep these two rules independent.
	preferred := make(map[string]bool, len(b.roles)+len(b.recent))
	for _, selector := range b.recent {
		preferred[selector] = true
	}
	for _, selector := range b.roles {
		base, _ := modelroles.Split(selector)
		preferred[base] = true
	}
	split := 0
	for split < len(ranked) && preferred[ranked[split].fq] {
		split++
	}
	if split > 0 && split < len(b.items) {
		b.items = slices.Insert(b.items, split, ModelSelectorItem{Kind: "separator"})
	}
}
