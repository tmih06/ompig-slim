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

// compareBrowserItems is OMP modelItemComparator: built-in role rank, MRU,
// provider, declared priority, version descending, latest/date, then locale ID.
// A live search skips role rank so a default cannot outrank a better match.
func (b *ModelBrowser) compareBrowserItems(a, c ModelSelectorItem, search bool) int {
	if a.Role != "" || c.Role != "" {
		return 0
	}
	if !search {
		rank := func(item ModelSelectorItem) int {
			for i, role := range modelroles.Builtins() {
				base, _ := modelroles.Split(b.roles[role])
				if base == item.FQ() {
					return i
				}
			}
			return len(modelroles.Builtins())
		}
		if ar, cr := rank(a), rank(c); ar != cr {
			return ar - cr
		}
	}
	recent := func(item ModelSelectorItem) int {
		i := slices.Index(b.recent, item.Provider+"/"+item.ID)
		if i < 0 {
			return math.MaxInt
		}
		return i
	}
	if ar, cr := recent(a), recent(c); ar != cr {
		if ar < cr {
			return -1
		}
		return 1
	}
	if cmp := localecompare.Compare(a.Provider, c.Provider); cmp != 0 {
		return cmp
	}
	priority := func(item ModelSelectorItem) float64 {
		if item.Model != nil && item.Model.Priority != nil {
			return *item.Model.Priority
		}
		return math.MaxFloat64
	}
	if ap, cp := priority(a), priority(c); ap != cp {
		if ap < cp {
			return -1
		}
		return 1
	}
	if av, cv := browserVersion(a.ID), browserVersion(c.ID); av != cv {
		if av > cv {
			return -1
		}
		return 1
	}
	al, cl := strings.HasSuffix(a.ID, "-latest"), strings.HasSuffix(c.ID, "-latest")
	ad, cd := "", ""
	if match := browserDatePattern.FindStringSubmatch(a.ID); match != nil {
		ad = match[1]
	}
	if match := browserDatePattern.FindStringSubmatch(c.ID); match != nil {
		cd = match[1]
	}
	if ar, cr := al || ad != "", cl || cd != ""; ar != cr {
		if ar {
			return -1
		}
		return 1
	}
	if al != cl {
		if al {
			return -1
		}
		return 1
	}
	if ad != "" && cd != "" && ad != cd {
		return strings.Compare(cd, ad)
	}
	return localecompare.Compare(a.ID, c.ID)
}

// rankBrowserItems seats a separator only above genuinely non-role/non-MRU
// catalog entries. Current selection is not a separate ranking rule in OMP.
func (b *ModelBrowser) rankBrowserItems() {
	b.items = slices.DeleteFunc(b.items, func(item ModelSelectorItem) bool { return item.Kind == "separator" })
	if len(b.items) == 0 || b.items[0].Role != "" {
		return
	}
	search := strings.TrimSpace(b.Query()) != ""
	slices.SortStableFunc(b.items, func(a, c ModelSelectorItem) int { return b.compareBrowserItems(a, c, search) })
	if search {
		return
	}
	preferred := func(item ModelSelectorItem) bool {
		if slices.Contains(b.recent, item.FQ()) {
			return true
		}
		for _, selector := range b.roles {
			base, _ := modelroles.Split(selector)
			if base == item.FQ() {
				return true
			}
		}
		return false
	}
	split := 0
	for split < len(b.items) && preferred(b.items[split]) {
		split++
	}
	if split > 0 && split < len(b.items) {
		b.items = slices.Insert(b.items, split, ModelSelectorItem{Kind: "separator"})
	}
}
