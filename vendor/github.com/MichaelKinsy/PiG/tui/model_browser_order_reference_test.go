package tui

import (
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/jsstring/localecompare"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// Frozen pre-optimization ordering oracle: preserve the source comparator and
// separator rules so precomputed keys cannot silently change tie precedence.
// referenceCompareBrowserItems is OMP modelItemComparator: built-in role rank, MRU,
// provider, declared priority, version descending, latest/date, then locale ID.
// A live search skips role rank so a default cannot outrank a better match.
func (b *ModelBrowser) referenceCompareBrowserItems(a, c ModelSelectorItem, search bool) int {
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

// referenceRankBrowserItems seats a separator only above genuinely non-role/non-MRU
// catalog entries. Current selection is not a separate ranking rule in OMP.
func (b *ModelBrowser) referenceRankBrowserItems() {
	b.items = slices.DeleteFunc(b.items, func(item ModelSelectorItem) bool { return item.Kind == "separator" })
	if len(b.items) == 0 || b.items[0].Role != "" {
		return
	}
	search := strings.TrimSpace(b.Query()) != ""
	slices.SortStableFunc(b.items, func(a, c ModelSelectorItem) int { return b.referenceCompareBrowserItems(a, c, search) })
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

// TestBrowserPrecomputedOrderMatchesReference compares the full catalog and
// adversarial ties with the previous implementation. Queries, duplicate MRU,
// shared built-in bindings, custom-role separators and virtual-role rows retain
// their original precedence; no lookup/render cache may stale these bindings.
func TestBrowserPrecomputedOrderMatchesReference(t *testing.T) {
	rows := ai.ListModels("")
	items := make([]ModelSelectorItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, ModelSelectorItem{Provider: row.Provider, ID: row.ID, Model: row.ToModel()})
	}
	items = append(items, ModelSelectorItem{Provider: "p", ID: "claude-sonnet-4-6"}, ModelSelectorItem{Provider: "P", ID: "same"}, ModelSelectorItem{Provider: "p", ID: "same"}, ModelSelectorItem{Provider: "p", ID: "gpt-5.4-latest"}, ModelSelectorItem{Kind: "separator"})
	for _, virtual := range []bool{false, true} {
		sample := slices.Clone(items)
		if virtual {
			sample = append(sample, ModelSelectorItem{Role: "review", Provider: "p", ID: "same"})
		}
		for _, query := range []string{"", "gpt", "claude", " "} {
			actual, reference := NewModelBrowser(""), NewModelBrowser("")
			roles := map[string]string{"default": "openai/gpt-4o:high", "vision": "openai/gpt-4o", "custom": "p/same", "smol": "openai/gpt-6-luna"}
			recent := []string{"openai/gpt-6-luna", "openai/gpt-4o", "openai/gpt-6-luna"}
			actual.roles, reference.roles = roles, roles
			actual.recent, reference.recent = recent, recent
			actual.items, reference.items = slices.Clone(sample), slices.Clone(sample)
			actual.search.SetText(query)
			reference.search.SetText(query)
			actual.rankBrowserItems()
			reference.referenceRankBrowserItems()
			if !reflect.DeepEqual(actual.items, reference.items) {
				t.Fatalf("ordering changed query=%q virtual=%v", query, virtual)
			}
		}
	}
}
