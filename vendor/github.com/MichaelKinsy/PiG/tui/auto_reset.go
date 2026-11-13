package tui

import (
	"sort"
	"strconv"
	"strings"
)

// AutoResetAccount is one stored account in the automatic-reset settings view.
type AutoResetAccount struct {
	ProviderID     string
	ProviderLabel  string
	Tier           string
	Label          string
	TargetRef      string
	Active         bool
	Enabled        bool
	ProviderLocked bool
	Err            string
}

type autoResetRow struct {
	provider string
	tier     string
	account  *AutoResetAccount
}

// AutoResetSelectorComponent is a grouped provider/tier/account checkbox list.
// Provider rows toggle every account, tier rows toggle accounts in that tier,
// and account rows toggle only themselves.
type AutoResetSelectorComponent struct {
	*Container
	accounts         []AutoResetAccount
	rows             []autoResetRow
	selected         int
	listContainer    *Container
	footerText       *Text
	onToggle         func(AutoResetAccount, bool)
	onProviderToggle func(provider string, enabled bool)
	onCancel         func()
	done             bool
	cancelled        bool
	themeList        SelectListTheme
}

// NewAutoResetSelectorComponent builds a nested provider/tier checkbox list.
// Accounts retain store order within each tier; providers and tiers are sorted.
func NewAutoResetSelectorComponent(accounts []AutoResetAccount, onToggle func(AutoResetAccount, bool), onCancel func()) *AutoResetSelectorComponent {
	copyAccounts := append([]AutoResetAccount(nil), accounts...)
	groups := map[string]map[string][]AutoResetAccount{}
	for index := range copyAccounts {
		account := &copyAccounts[index]
		if strings.TrimSpace(account.Tier) == "" {
			account.Tier = "Unknown"
		}
		if groups[account.ProviderID] == nil {
			groups[account.ProviderID] = map[string][]AutoResetAccount{}
		}
		groups[account.ProviderID][account.Tier] = append(groups[account.ProviderID][account.Tier], *account)
	}
	providers := make([]string, 0, len(groups))
	for provider := range groups {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	rows := make([]autoResetRow, 0, len(copyAccounts)*2+len(providers))
	for _, provider := range providers {
		rows = append(rows, autoResetRow{provider: provider})
		tiers := make([]string, 0, len(groups[provider]))
		for tier := range groups[provider] {
			tiers = append(tiers, tier)
		}
		sort.Strings(tiers)
		for _, tier := range tiers {
			rows = append(rows, autoResetRow{provider: provider, tier: tier})
			for index := range groups[provider][tier] {
				account := groups[provider][tier][index]
				rows = append(rows, autoResetRow{provider: provider, tier: tier, account: &account})
			}
		}
	}
	selector := &AutoResetSelectorComponent{Container: NewContainer(), accounts: copyAccounts, rows: rows, onToggle: onToggle, onCancel: onCancel, themeList: GetSelectListTheme()}
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

// SetProviderToggleHandler sets the persistence callback for provider lock changes.
func (s *AutoResetSelectorComponent) SetProviderToggleHandler(handler func(provider string, enabled bool)) {
	s.onProviderToggle = handler
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
		switch {
		case row.account != nil:
			items = append(items, SelectItem{Value: row.account.ProviderID + "\x00" + row.account.TargetRef, Label: "    " + checkbox(row.account.Enabled) + " " + row.account.Label, Description: autoResetAccountDescription(*row.account)})
		case row.tier != "":
			items = append(items, SelectItem{Value: row.provider + "\x00" + row.tier, Label: "  " + checkbox(s.tierEnabled(row.provider, row.tier)) + " " + row.tier, Description: s.tierDescription(row.provider, row.tier)})
		default:
			items = append(items, SelectItem{Value: row.provider, Label: checkbox(s.providerLocked(row.provider)) + " " + s.providerLabel(row.provider), Description: s.providerDescription(row.provider)})
		}
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

func (s *AutoResetSelectorComponent) providerLocked(provider string) bool {
	for _, account := range s.accounts {
		if account.ProviderID == provider {
			return account.ProviderLocked
		}
	}
	return false
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
	return strconv.Itoa(enabled) + "/" + strconv.Itoa(count) + " accounts enabled · toggle all"
}

func (s *AutoResetSelectorComponent) tierEnabled(provider, tier string) bool {
	found := false
	for _, account := range s.accounts {
		if account.ProviderID == provider && account.Tier == tier {
			found = true
			if !account.Enabled {
				return false
			}
		}
	}
	return found
}

func (s *AutoResetSelectorComponent) tierDescription(provider, tier string) string {
	count, enabled := 0, 0
	for _, account := range s.accounts {
		if account.ProviderID == provider && account.Tier == tier {
			count++
			if account.Enabled {
				enabled++
			}
		}
	}
	return strconv.Itoa(enabled) + "/" + strconv.Itoa(count) + " accounts enabled · toggle tier"
}

func (s *AutoResetSelectorComponent) footer() string {
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
	case kb.Matches(data, KBSelectConfirm) || data == "\n" || data == " ":
		s.toggle()
	case data == "l":
		s.lockProvider()
	}
}

func (s *AutoResetSelectorComponent) move(delta int) {
	if len(s.rows) == 0 {
		return
	}
	s.selected = max(0, min(s.selected+delta, len(s.rows)-1))
	s.rebuild()
}

func (s *AutoResetSelectorComponent) toggle() {
	if s.selected < 0 || s.selected >= len(s.rows) {
		return
	}
	row := s.rows[s.selected]
	if row.account != nil {
		s.setAccounts(row.provider, row.tier, row.account.TargetRef, !row.account.Enabled)
	} else if row.tier != "" {
		s.setAccounts(row.provider, row.tier, "", !s.tierEnabled(row.provider, row.tier))
	} else {
		s.setAccounts(row.provider, "", "", !s.providerEnabled(row.provider))
	}
	s.rebuild()
}

func (s *AutoResetSelectorComponent) lockProvider() {
	if s.selected < 0 || s.selected >= len(s.rows) {
		return
	}
	provider := s.rows[s.selected].provider
	enabled := !s.providerLocked(provider)
	s.setProviderLocked(provider, enabled)
	s.setAccounts(provider, "", "", enabled)
	if s.onProviderToggle != nil {
		s.onProviderToggle(provider, enabled)
	}
	s.rebuild()
}

func (s *AutoResetSelectorComponent) setProviderLocked(provider string, locked bool) {
	for index := range s.accounts {
		if s.accounts[index].ProviderID == provider {
			s.accounts[index].ProviderLocked = locked
		}
	}
}

func (s *AutoResetSelectorComponent) setAccounts(provider, tier, target string, enabled bool) {
	for index := range s.accounts {
		account := &s.accounts[index]
		if account.ProviderID != provider || (tier != "" && account.Tier != tier) || (target != "" && account.TargetRef != target) {
			continue
		}
		account.Enabled = enabled
		for rowIndex := range s.rows {
			row := &s.rows[rowIndex]
			if row.account != nil && row.account.ProviderID == provider && row.account.Tier == account.Tier && row.account.TargetRef == account.TargetRef {
				row.account.Enabled = enabled
			}
		}
		if s.onToggle != nil {
			s.onToggle(*account, enabled)
		}
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
		return account.ProviderLabel + " · " + account.Tier + " · active account"
	}
	return account.ProviderLabel + " · " + account.Tier
}
