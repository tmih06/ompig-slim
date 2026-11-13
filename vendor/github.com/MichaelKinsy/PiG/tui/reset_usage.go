package tui

// Ports packages/tui/src/overlays/reset-usage-selector.ts: the account picker
// behind `/usage reset`. Each row is one stored account with its saved-reset
// count, and spending one asks for a second confirmation because a reset is a
// scarce, irreversible credit.

import (
	"strconv"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// ResetUsageAccount is one selectable account row.
type ResetUsageAccount struct {
	// Label is the account identity (email, account id, or key hint).
	Label string
	// ProviderID is the provider id; ProviderLabel is its display name.
	ProviderID    string
	ProviderLabel string
	// AvailableCount is the banked inventory, RedeemableCount what may be spent
	// now.
	AvailableCount  int
	RedeemableCount int
	// TargetRef is the typed `<provider>/<account>` reference the row spends,
	// used by the listing and by headless output.
	TargetRef string
	Active    bool
	AutoReset bool
	// Err reports an account whose inventory could not be read.
	Err string
	// UnavailableReason explains a banked inventory that cannot be spent now.
	UnavailableReason string
	// ExpiresAt is the soonest expiry among the banked resets, as RFC 3339.
	ExpiresAt string
	// CreditTitle names the credit a spend would use, when the provider names it.
	CreditTitle string
	// CreditID is that credit's id. A spend passes it back so the provider spends
	// the same grant the row described, instead of re-picking one.
	CreditID string
}

const resetUsageVisibleRows = 10

// ResetUsageSelectorComponent is the editor-slot account picker for `/usage
// reset`. Construct it with NewResetUsageSelectorComponent and drive it through
// HandleInput; Done reports when the host may restore the editor.
type ResetUsageSelectorComponent struct {
	*Container
	accounts      []ResetUsageAccount
	selected      int
	pending       int // index awaiting confirmation, -1 when none
	status        string
	listContainer *Container
	footerText    *Text
	onSelect      func(account ResetUsageAccount)
	onCancel      func()
	done          bool
	cancelled     bool
	themeList     SelectListTheme
}

// RedeemableResetUsageAccounts returns a new slice containing only accounts
// with banked resets usable now. Failed reads and expired or malformed expiry
// stamps cannot authorize a spend. Undated credits remain valid when the
// provider reports them usable. Input order and the original slice are preserved.
func RedeemableResetUsageAccounts(accounts []ResetUsageAccount) []ResetUsageAccount {
	now := time.Now()
	valid := make([]ResetUsageAccount, 0, len(accounts))
	for _, account := range accounts {
		if account.Err != "" || account.AvailableCount <= 0 || account.RedeemableCount <= 0 {
			continue
		}
		if account.ExpiresAt != "" {
			expiry, err := time.Parse(time.RFC3339Nano, account.ExpiresAt)
			if err != nil || !expiry.After(now) {
				continue
			}
		}
		valid = append(valid, account)
	}
	return valid
}

// NewResetUsageSelectorComponent builds a picker of usable accounts only. onSelect runs after the
// second confirmation; onCancel runs on the cancel key with no pending
// confirmation. Both may be nil for a host that polls Done.
func NewResetUsageSelectorComponent(accounts []ResetUsageAccount, onSelect func(ResetUsageAccount), onCancel func()) *ResetUsageSelectorComponent {
	selector := &ResetUsageSelectorComponent{
		Container: NewContainer(),
		accounts:  RedeemableResetUsageAccounts(accounts),
		selected:  0,
		pending:   -1,
		onSelect:  onSelect,
		onCancel:  onCancel,
		themeList: GetSelectListTheme(),
	}
	title := NewText(ActiveTheme().Bold(ActiveTheme().Fg("accent", "Spend a saved rate-limit reset")))
	selector.listContainer = NewContainer()
	selector.footerText = NewPaddedText("", 0, 0, nil)
	selector.Add(NewDynamicBorder())
	selector.Add(NewSpacer(1))
	selector.Add(title)
	selector.Add(NewSpacer(1))
	selector.Add(selector.listContainer)
	selector.Add(NewSpacer(1))
	selector.Add(selector.footerText)
	selector.Add(NewDynamicBorder())
	selector.rebuild()
	return selector
}

// Done reports whether the picker finished, by spending or by cancelling.
func (s *ResetUsageSelectorComponent) Done() bool { return s.done }

// Cancelled reports whether the picker finished without spending.
func (s *ResetUsageSelectorComponent) Cancelled() bool { return s.cancelled }

// Accounts returns the rows the picker lists.
func (s *ResetUsageSelectorComponent) Accounts() []ResetUsageAccount { return s.accounts }

// ResetUsageTargetRef renders the typed reference of one row: the account key
// when the store has one, else "active" for the provider's selected account,
// which is how an identity-less account is addressed back.
func ResetUsageTargetRef(account ResetUsageAccount) string {
	reference := account.TargetRef
	if reference == "" && account.Active {
		reference = "active"
	}
	return account.ProviderID + "/" + reference
}

// rebuild redraws the list, the pending strip and the hint. It runs on every
// selection change so the confirmation and the account move together.
func (s *ResetUsageSelectorComponent) rebuild() {
	items := make([]SelectItem, 0, len(s.accounts))
	for _, account := range s.accounts {
		items = append(items, SelectItem{
			Value:       accountLabelValue(account),
			Label:       resetUsageRowLabel(account),
			Description: resetUsageRowDescription(account),
		})
	}
	list := NewSelectList(items, resetUsageVisibleRows, s.themeList)
	list.SetSelectedIndex(s.selected)
	s.listContainer.SetChildren(list)
	s.footerText.SetText(s.footer())
	s.Invalidate()
}

// footer renders the confirmation question or the navigation hint.
func (s *ResetUsageSelectorComponent) footer() string {
	theme := ActiveTheme()
	if s.status != "" {
		return theme.Fg("warning", s.status)
	}
	if s.pending >= 0 && s.pending < len(s.accounts) {
		account := s.accounts[s.pending]
		subject := "1 saved reset"
		if account.CreditTitle != "" {
			subject = "“" + account.CreditTitle + "”"
		}
		return theme.Fg("warning", "Press "+ActionKeyDisplayText(KBSelectConfirm)+" again to spend "+subject+
			" for "+account.Label+" ("+account.ProviderLabel+"). "+ActionKeyDisplayText(KBSelectCancel)+" cancels.")
	}
	return theme.Fg("dim", ActionKeyDisplayText(KBSelectUp)+"/"+ActionKeyDisplayText(KBSelectDown)+" select · "+
		ActionKeyDisplayText(KBSelectConfirm)+" spend a reset · "+ActionKeyDisplayText(KBSelectCancel)+" cancel")
}

// HandleInput resolves navigation, confirmation and cancellation in that order.
// A confirm on an account nothing can be spent for reports why instead of
// opening a confirmation the account cannot satisfy.
func (s *ResetUsageSelectorComponent) HandleInput(data string) {
	kb := GetTUIKeybindings()
	switch {
	case kb.Matches(data, KBSelectCancel):
		if s.pending >= 0 {
			s.pending = -1
			s.status = ""
			s.rebuild()
			return
		}
		s.done = true
		s.cancelled = true
		if s.onCancel != nil {
			s.onCancel()
		}
	case kb.Matches(data, KBSelectUp):
		s.move(-1)
	case kb.Matches(data, KBSelectDown):
		s.move(1)
	case kb.Matches(data, KBSelectPageUp):
		s.move(-resetUsageVisibleRows)
	case kb.Matches(data, KBSelectPageDown):
		s.move(resetUsageVisibleRows)
	case kb.Matches(data, KBSelectConfirm) || data == "\n":
		s.activate()
	}
}

// move changes the highlighted row and drops a pending confirmation, because
// the question belonged to the previous row.
func (s *ResetUsageSelectorComponent) move(delta int) {
	if len(s.accounts) == 0 {
		return
	}
	s.pending = -1
	s.status = ""
	s.selected = max(0, min(s.selected+delta, len(s.accounts)-1))
	s.rebuild()
}

// activate asks for confirmation, then spends on the second press.
func (s *ResetUsageSelectorComponent) activate() {
	if s.selected < 0 || s.selected >= len(s.accounts) {
		return
	}
	account := s.accounts[s.selected]
	if account.Err != "" || account.RedeemableCount <= 0 {
		reason := account.UnavailableReason
		if account.Err != "" {
			reason = account.Err
		}
		if reason == "" {
			reason = "no saved resets usable right now"
		}
		s.pending = -1
		s.status = "That account's saved resets are unavailable: " + reason
		s.rebuild()
		return
	}
	if s.pending != s.selected {
		s.pending = s.selected
		s.status = ""
		s.rebuild()
		return
	}
	s.done = true
	s.pending = -1
	if s.onSelect != nil {
		s.onSelect(account)
	}
}

// accountLabelValue is the stable identity of a row, used by the list's filter.
func accountLabelValue(account ResetUsageAccount) string {
	ref := ResetUsageTargetRef(account)
	if account.Label == "" {
		return ref
	}
	return account.Label + " " + ref
}

// resetUsageRowLabel renders the primary column of one row: the account
// identity, which is what the user recognizes. The provider and the typed
// reference follow in the description so a narrow column never truncates them
// away from the account name.
func resetUsageRowLabel(account ResetUsageAccount) string {
	theme := ActiveTheme()
	name := SanitizeDisplayLine(account.Label)
	if account.RedeemableCount <= 0 || account.Err != "" {
		return theme.Fg("dim", name)
	}
	return name
}

// resetUsageRowDescription renders the count column: the provider and typed
// reference, banked resets, the subset usable now, the soonest expiry as a
// countdown, and why nothing is spendable. Everything here is one row wide, so
// the expiry is a duration rather than a timestamp.
func resetUsageRowDescription(account ResetUsageAccount) string {
	theme := ActiveTheme()
	mark := SanitizeDisplayLine(account.ProviderLabel) + " · " + SanitizeDisplayLine(account.TargetRef)
	if account.Err != "" {
		return theme.Fg("error", mark+" · unavailable: "+SanitizeDisplayLine(account.Err))
	}
	if account.Active {
		mark += " (active)"
	}
	parts := []string{mark, resetUsageCountLabel(account)}
	if expires := resetUsageExpiryLabel(account.ExpiresAt, time.Now().UnixMilli()); expires != "" {
		parts = append(parts, expires)
	}
	if account.RedeemableCount <= 0 && account.UnavailableReason != "" {
		parts = append(parts, SanitizeDisplayLine(account.UnavailableReason))
	}
	color := "success"
	if account.RedeemableCount <= 0 {
		color = "dim"
	}
	return theme.Fg(color, strings.Join(parts, " · "))
}

// resetUsageCountLabel renders the banked inventory and, when the provider
// permits less, the subset spendable now. The distinction is the point of the
// row: a banked reset the provider refuses is not a reset the user can spend.
func resetUsageCountLabel(account ResetUsageAccount) string {
	saved := strconv.Itoa(account.AvailableCount) + " saved"
	if account.RedeemableCount == account.AvailableCount {
		return saved
	}
	return saved + " · " + strconv.Itoa(account.RedeemableCount) + " usable now"
}

// resetUsageExpiryLabel renders the soonest banked expiry as a countdown. An
// expired stamp still reads as expired, because a banked-but-expired credit is
// what the account's provider will refuse.
func resetUsageExpiryLabel(expiresAt string, nowMs int64) string {
	if expiresAt == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return ""
	}
	remaining := parsed.UnixMilli() - nowMs
	if remaining <= 0 {
		return "expired"
	}
	return "expires in " + FormatDuration(remaining)
}

// ResetUsageSummaryLine renders one row as a listing line, used by the
// transcript listing and headless output.
func ResetUsageSummaryLine(account ResetUsageAccount) string {
	detail := strconv.Itoa(account.AvailableCount) + " saved, " + strconv.Itoa(account.RedeemableCount) + " usable now"
	if account.Err != "" {
		detail = "unavailable (" + SanitizeDisplayLine(account.Err) + ")"
	} else if account.RedeemableCount == 0 && account.UnavailableReason != "" {
		detail += " (" + SanitizeDisplayLine(account.UnavailableReason) + ")"
	}
	if expires := resetUsageExpiryLabel(account.ExpiresAt, time.Now().UnixMilli()); expires != "" {
		detail += ", " + expires
	}
	active := ""
	if account.Active {
		active = " (active)"
	}
	return "- " + SanitizeDisplayLine(account.Label) + " [" + SanitizeDisplayLine(account.ProviderLabel) +
		" · " + ResetUsageTargetRef(account) + "]: " + detail + active
}

// ResetCreditSummaryLines renders a saved-reset inventory through the shared
// display summary: banked count, the subset spendable now, the soonest expiry
// as an RFC 3339 timestamp, and why nothing is spendable.
func ResetCreditSummaryLines(credits *ai.UsageResetCredits, nowMs int64) (banked, redeemable int, expiresAt, reason string) {
	summary := SummarizeUsageResetCredits(credits, nowMs)
	if summary == nil {
		return 0, 0, "", ""
	}
	return summary.BankedCount, summary.RedeemableCount, summary.SoonestExpiry, summary.UnavailableReason
}
