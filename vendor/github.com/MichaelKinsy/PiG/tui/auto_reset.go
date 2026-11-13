package tui

import (
	"sort"
	"strconv"
	"strings"
)

// AutoResetAccount is one stored account in the automatic-reset settings view.
type AutoResetAccount struct {
	ProviderID    string
	ProviderLabel string
	Label         string
	TargetRef     string
	Active        bool
	Enabled       bool
	Err           string
}

type autoResetRow struct {
	provider string
	account  *AutoResetAccount
}

// AutoResetSelectorComponent is a grouped provider/account checkbox list. A
// provider row toggles every account in that provider; an account row toggles
// only itself. Changes are reported immediately and the view stays open so a
// user can configure several accounts in one visit.
type AutoResetSelectorComponent struct {
	*Container
	accounts      []AutoResetAccount
	rows          []autoResetRow
	selected      int
	status        string
	listContainer *Container
	footerText    *Text
	onToggle      func(AutoResetAccount, bool)
	onCancel      func()
	done          bool
	cancelled     bool
	themeList     SelectListTheme
}

// NewAutoResetSelectorComponent builds a grouped checkbox list from stored
// accounts. Accounts are grouped by provider and retain store order within each
// provider; provider groups themselves are sorted by provider id.
func NewAutoResetSelectorComponent(accounts []AutoResetAccount, onToggle func(AutoResetAccount, bool), onCancel func()) *AutoResetSelectorComponent {
	copyAccounts := append([]AutoResetAccount(nil), accounts...)
	providers := map[string][]AutoResetAccount{}
	for _, account := range copyAccounts {
		providers[account.ProviderID] = append(providers[account.ProviderID], account)
	}
	ids := make([]string, 0, len(providers))
	for provider := range providers {
		ids = append(ids, provider)
	}
	sort.Strings(ids)
	rows := make([]autoResetRow, 0, len(copyAccounts)+len(ids))
	for _, provider := range ids {
		rows = append(rows, autoResetRow{provider: provider})
		for index := range providers[provider] {
			account := providers[provider][index]
			rows = append(rows, autoResetRow{provider: provider, account: &account})
		}
	}
	selector := &AutoResetSelectorComponent{
		Container: NewContainer(), accounts: copyAccounts, rows: rows,
		onToggle: onToggle, onCancel: onCancel, themeList: GetSelectListTheme(),
	}
	selector.listContainer = NewContainer()
	selector.footerText = NewPaddedText("", 0, 0, nil)
	selector.Add(NewDynamicBorder())
	selector.Add(NewSpacer(1))
	selector.Add(NewText(ActiveTheme().Bold(ActiveTheme().Fg("accent", "Automatic rate-limit resets"))))
	selector.Add(NewSpacer(1))
	selector.Add(selector.listContainer)
	selector.Add(NewSpacer(1))
	selector.Add(selector.footerText)
	selector.Add(NewDynamicBorder())
	selector.rebuild()
	return selector
}

// Done reports whether the settings view has been cancelled or closed.
func (s *AutoResetSelectorComponent) Done() bool { return s.done }

// Cancelled reports whether the view ended without a normal confirmation.
func (s *AutoResetSelectorComponent) Cancelled() bool { return s.cancelled }

// Accounts returns the current account preferences in provider/store order.
func (s *AutoResetSelectorComponent) Accounts() []AutoResetAccount { return s.accounts }

func (s *AutoResetSelectorComponent) rebuild() {
	items := make([]SelectItem, 0, len(s.rows))
	for _, row := range s.rows {
		if row.account == nil {
			items = append(items, SelectItem{Value: row.provider, Label: checkbox(s.providerEnabled(row.provider)) + " " + s.providerLabel(row.provider), Description: s.providerDescription(row.provider)})
			continue
		}
		items = append(items, SelectItem{Value: row.account.ProviderID + "\x00" + row.account.TargetRef, Label: "  " + checkbox(row.account.Enabled) + " " + row.account.Label, Description: autoResetAccountDescription(*row.account)})
	}
	list := NewSelectList(items, 10, s.themeList)
	list.SetSelectedIndex(s.selected)
	s.listContainer.SetChildren(list)
	s.footerText.SetText(s.footer())
	s.Invalidate()
}

func (s *AutoResetSelectorComponent) providerLabel(provider string) string {
	for _, account := range s.accounts {
		if account.ProviderID == provider && account.ProviderLabel != "" {
			return account.ProviderLabel
		}
	}
	return provider
}

func (s *AutoResetSelectorComponent) providerEnabled(provider string) bool {
	found := false
	for _, account := range s.accounts {
		if account.ProviderID == provider {
			found = true
			if !account.Enabled {
				return false
			}
		}
	}
	return found
}

func (s *AutoResetSelectorComponent) providerDescription(provider string) string {
	count, enabled := 0, 0
	for _, account := range s.accounts {
		if account.ProviderID == provider {
			count++
			if account.Enabled {
				enabled++
			}
		}
	}
	return strings.Join([]string{strconv.Itoa(enabled) + "/" + strconv.Itoa(count) + " accounts enabled", "toggle all"}, " · ")
}

func (s *AutoResetSelectorComponent) footer() string {
	if s.status != "" {
		return ActiveTheme().Fg("warning", s.status)
	}
	return ActiveTheme().Fg("dim", ActionKeyDisplayText(KBSelectUp)+"/"+ActionKeyDisplayText(KBSelectDown)+" select · "+ActionKeyDisplayText(KBSelectConfirm)+" toggle · "+ActionKeyDisplayText(KBSelectCancel)+" close")
}

// HandleInput handles navigation, toggles, and close without requiring a
// second confirmation: enabling this feature is reversible and account-scoped.
func (s *AutoResetSelectorComponent) HandleInput(data string) {
	kb := GetTUIKeybindings()
	switch {
	case kb.Matches(data, KBSelectCancel):
		s.done, s.cancelled = true, true
		if s.onCancel != nil {
			s.onCancel()
		}
	case kb.Matches(data, KBSelectUp):
		s.move(-1)
	case kb.Matches(data, KBSelectDown):
		s.move(1)
	case kb.Matches(data, KBSelectPageUp):
		s.move(-10)
	case kb.Matches(data, KBSelectPageDown):
		s.move(10)
	case kb.Matches(data, KBSelectConfirm) || data == "\n":
		s.toggle()
	}
}

func (s *AutoResetSelectorComponent) move(delta int) {
	if len(s.rows) == 0 {
		return
	}
	s.selected = max(0, min(s.selected+delta, len(s.rows)-1))
	s.status = ""
	s.rebuild()
}

func (s *AutoResetSelectorComponent) toggle() {
	if s.selected < 0 || s.selected >= len(s.rows) {
		return
	}
	row := s.rows[s.selected]
	if row.account != nil {
		s.setAccount(row.account.ProviderID, row.account.TargetRef, !row.account.Enabled)
		s.status = ""
		s.rebuild()
		return
	}
	allEnabled := true
	found := false
	for _, account := range s.accounts {
		if account.ProviderID == row.provider {
			found = true
			allEnabled = allEnabled && account.Enabled
		}
	}
	if !found {
		return
	}
	for _, account := range s.accounts {
		if account.ProviderID == row.provider {
			s.setAccount(account.ProviderID, account.TargetRef, !allEnabled)
		}
	}
	s.status = ""
	s.rebuild()
}

func (s *AutoResetSelectorComponent) setAccount(provider, target string, enabled bool) {
	for index := range s.accounts {
		if s.accounts[index].ProviderID != provider || s.accounts[index].TargetRef != target {
			continue
		}
		s.accounts[index].Enabled = enabled
		for rowIndex := range s.rows {
			if s.rows[rowIndex].account != nil && s.rows[rowIndex].account.ProviderID == provider && s.rows[rowIndex].account.TargetRef == target {
				s.rows[rowIndex].account.Enabled = enabled
			}
		}
		if s.onToggle != nil {
			s.onToggle(s.accounts[index], enabled)
		}
		return
	}
}

func checkbox(enabled bool) string {
	if enabled {
		return "☑"
	}
	return "☐"
}

func autoResetAccountDescription(account AutoResetAccount) string {
	if account.Err != "" {
		return "unavailable: " + account.Err
	}
	if account.Active {
		return account.ProviderLabel + " · active account"
	}
	return account.ProviderLabel
}
