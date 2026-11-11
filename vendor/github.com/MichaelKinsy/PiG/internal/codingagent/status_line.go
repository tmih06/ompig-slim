package codingagent

import (
	"context"
	"fmt"
	"maps"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/jsnumber"
	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/internal/jsstring/localecompare"
	"github.com/MichaelKinsy/PiG/internal/nodepath"
	"github.com/MichaelKinsy/PiG/internal/nodespawn"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// FooterComponent is the rich footer rendered at the bottom of the
// interactive viewport. It renders two lines built from upstream pi's
// FooterComponent (footer.ts), with the model moved up beside the project:
//
//	Line 1: ~/<pwd> (<branch>) • <session-name>                  <model> • <thinking>
//	Line 2: ↑<in> ↓<out> [R<cache>] [W<cache>] [$<cost>] <context%>/<window> (auto)
//
// Color coding on the context-% column (upstream thresholds):
//
//	<=70%  default (no color)
//	>70%   yellow (warning)
//	>90%   red (error)
//
// All inputs are read on every Render() so the line reflects live state.
type FooterComponent struct {
	tui.BaseComponent

	mu sync.RWMutex

	model                  *ai.Model
	agentName              string
	timings                *agent.Recorder
	contextTokens          int // last turn's total tokens (input+output+cacheRead+cacheWrite) for context%
	contextUnknown         bool
	projectedContextWindow int
	working                bool

	// usageTotals reads the session's all-entry usage totals on each render,
	// as upstream footer.ts sums every session entry's stored usage.
	usageTotals func() footerUsageTotals
	// routedModel reads, under a virtual model selection, the physical model of the latest response on each render (footer.ts reads session.routedModel).
	routedModel func() *RoutedModelSelection
	// activeRole reads the logical role of the current selection and the theme token the model catalog paints it with, on each render. It is a
	// callback rather than a SetRole setter because every model change already rebinds the footer, and a role can change without the physical model
	// changing (fallback restoration, a cycle onto another role that shares the model), so no single setter call would keep it current.
	activeRole func() (role, color string)
	// subscriptionFor decides the "(sub)" marker for a newly bound model.
	subscriptionFor func(*ai.Model) bool

	// FooterDataProvider holds the cwd, git branch, extension statuses and provider count the footer reads on each render
	// (footer.ts footerData; footer-data-provider.ts).
	*FooterDataProvider

	// Session name shown in footer as " • <name>".
	name string

	// Thinking level display on line 2 right side.
	thinkingLevel string
	// Auto-compact indicator "(auto)" shown next to context %.
	autoCompactEnabled bool

	// customWorkingMessage: extension-set message shown during streaming.
	// Empty = use default spinner. Mirrors upstream loadingAnimation.setMessage.
	customWorkingMessage string

	// usingSubscription: OAuth subscription pricing (e.g. GitHub Copilot).
	// Shows "(sub)" in cost display. Mirrors upstream footer.ts:128.
	usingSubscription bool

	// suppressedByExtFooter hides the standard footer. The custom footer owns
	// keyed status presentation through its FooterData snapshot.
	suppressedByExtFooter bool
}

// NewFooterComponent creates a FooterComponent bound to the given model + agent.
// timings may be nil; cost/elapsed columns are then suppressed.
func NewFooterComponent(model *ai.Model, agentName string, timings *agent.Recorder) *FooterComponent {
	if agentName == "" {
		agentName = "default"
	}
	s := &FooterComponent{
		model:              model,
		agentName:          agentName,
		timings:            timings,
		autoCompactEnabled: true, // default matches upstream
		FooterDataProvider: NewFooterDataProvider(),
	}
	return s
}

// NewFooterComponentForSession is Pi's FooterComponent constructor (footer.ts:68-71): a footer that reads usage totals and the routed model
// from session and its cwd, git branch, extension statuses and provider count from footerData. The model is set with SetModel.
func NewFooterComponentForSession(session FooterSession, footerData *FooterDataProvider) *FooterComponent {
	s := NewFooterComponent(nil, "", nil)
	if footerData != nil {
		s.FooterDataProvider = footerData
	}
	s.SetSession(session)
	return s
}

// GitBranch returns the cached git branch, empty outside a repo.
func (s *FooterComponent) GitBranch() string { return s.GetGitBranch() }

// ProviderCount returns the number of authenticated, reachable providers.
func (s *FooterComponent) ProviderCount() int { return s.GetAvailableProviderCount() }

// SetCwd binds the footer and its branch watcher to the repository present at initialization.
func (s *FooterComponent) SetCwd(cwd string) {
	s.FooterDataProvider.SetCwd(cwd)
	s.Invalidate()
}

// SetWorking flips the spinner column on/off.
func (s *FooterComponent) SetWorking(b bool) {
	s.mu.Lock()
	s.working = b
	s.mu.Unlock()
	s.Invalidate()
}

// SetAgentName updates the agent persona label.
func (s *FooterComponent) SetAgentName(name string) {
	s.mu.Lock()
	s.agentName = name
	s.mu.Unlock()
	s.Invalidate()
}

// SetModel rebinds the model. Upstream footer.ts derives the subscription
// marker from the active model on every render, so a rebind re-evaluates it.
func (s *FooterComponent) SetModel(m *ai.Model) {
	s.mu.RLock()
	subscriptionFor := s.subscriptionFor
	s.mu.RUnlock()
	usingSubscription := false
	if subscriptionFor != nil {
		usingSubscription = subscriptionFor(m)
	}
	s.mu.Lock()
	s.model = m
	if subscriptionFor != nil {
		s.usingSubscription = usingSubscription
	}
	s.mu.Unlock()
	s.Invalidate()
}

// SetSubscriptionResolver installs the "(sub)" decision used by SetModel.
func (s *FooterComponent) SetSubscriptionResolver(resolve func(*ai.Model) bool) {
	s.mu.Lock()
	s.subscriptionFor = resolve
	s.mu.Unlock()
}

// FooterSession is the members of Pi's AgentSession that FooterComponent reads (footer.ts:103-107, :241): sessionManager, whose entries carry the
// all-entry usage totals (getSessionStats), routedModel, the physical model the latest response was routed to, and activeRole, the logical role the
// selection belongs to. Go's AgentSession is coding.Session, which imports this package, so the footer takes this consumer-owned interface of exactly
// those members, and package coding asserts that *coding.Session implements it.
type FooterSession interface {
	// SessionManager is `sessionManager`; a nil manager has no usage.
	SessionManager() *Session
	// RoutedModelSelection is `routedModel` in the form the footer renders: nil unless a virtual model selection routed the latest response.
	RoutedModelSelection() *RoutedModelSelection
	// ActiveModelRole is the logical role of the current selection, or "" for none. Two roles can share one physical model, so the model name alone
	// cannot tell them apart; the footer shows the role beside it.
	ActiveModelRole() string
	// ActiveModelRoleColor is the theme color token for that role, resolved from the same role metadata the model manager paints with, so the footer
	// and the catalog never disagree about a role's color. An empty token, or one the theme does not define, falls back to the footer's own hue.
	ActiveModelRoleColor() string
}

// SetSession points the footer at another session (footer.ts setSession); the next render reads that session's totals and routed model.
// A nil session reads neither.
func (s *FooterComponent) SetSession(session FooterSession) {
	s.mu.Lock()
	if session == nil {
		s.usageTotals, s.routedModel, s.activeRole = nil, nil, nil
	} else {
		s.usageTotals = func() footerUsageTotals {
			manager := session.SessionManager()
			if manager == nil {
				return footerUsageTotals{}
			}
			return manager.FooterUsageTotals()
		}
		s.routedModel = session.RoutedModelSelection
		s.activeRole = func() (string, string) { return session.ActiveModelRole(), session.ActiveModelRoleColor() }
	}
	s.mu.Unlock()
	s.Invalidate()
}

// SetName updates the session name shown in the footer.
func (s *FooterComponent) SetName(name string) {
	s.mu.Lock()
	s.name = name
	s.mu.Unlock()
	s.Invalidate()
}

// SetThinkingLevel updates the thinking level display.
func (s *FooterComponent) SetThinkingLevel(level string) {
	s.mu.Lock()
	s.thinkingLevel = level
	s.mu.Unlock()
	s.Invalidate()
}

// SetAutoCompactEnabled updates the "(auto)" indicator.
func (s *FooterComponent) SetAutoCompactEnabled(enabled bool) {
	s.mu.Lock()
	s.autoCompactEnabled = enabled
	s.mu.Unlock()
	s.Invalidate()
}

// SetProviderCount updates the number of authenticated+reachable providers.
// When >1, the footer shows "(provider) model" instead of just "model".
// Mirrors upstream footer.ts:165-170.
func (s *FooterComponent) SetProviderCount(n int) {
	s.FooterDataProvider.SetProviderCount(n)
	s.Invalidate()
}

// SetUsingSubscription updates the OAuth subscription indicator.
// When true, cost display shows "(sub)". Mirrors upstream footer.ts:128.
func (s *FooterComponent) SetUsingSubscription(v bool) {
	s.mu.Lock()
	s.usingSubscription = v
	s.mu.Unlock()
	s.Invalidate()
}

// SetExtensionStatus sets (or clears) a keyed status entry in the footer's
// extension-status line. Mirrors upstream footer-data-provider.ts setStatus.
// Pass empty text to remove the key.
func (s *FooterComponent) SetExtensionStatus(key, text string) {
	s.FooterDataProvider.SetExtensionStatus(key, text)
	s.Invalidate()
}

// SetTurnContextUsage records the latest usage for the context-window column.
// Token and cost totals come from the usage totals source instead.
func (s *FooterComponent) SetTurnContextUsage(u *ai.Usage) {
	if u == nil {
		return
	}
	s.mu.Lock()
	// Context tokens = this turn's total (represents actual context window
	// usage). Upstream footer.ts uses session.getContextUsage() which calls
	// estimateContextTokens → calculateContextTokens(lastAssistant.usage)
	// = usage.totalTokens || (input + output + cacheRead + cacheWrite).
	s.contextTokens = u.Input + u.Output + u.CacheRead + u.CacheWrite
	s.mu.Unlock()
	s.Invalidate()
}

// ResetContextUsage clears the context-window column before a transcript
// rebuild re-reads it from the branch.
func (s *FooterComponent) ResetContextUsage() {
	s.mu.Lock()
	s.contextTokens = 0
	s.mu.Unlock()
	s.Invalidate()
}

// SetWorkingMessage sets a custom message shown during streaming.
// Pass empty to restore the default spinner. Mirrors upstream
// loadingAnimation.setMessage (interactive-mode.ts:1877-1885).
func (s *FooterComponent) SetWorkingMessage(message string) {
	s.mu.Lock()
	s.customWorkingMessage = message
	s.mu.Unlock()
	s.Invalidate()
}

// GetWorkingMessage returns the current custom working message or "".
func (s *FooterComponent) GetWorkingMessage() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.customWorkingMessage
}

// SetSuppressedByExtFooter controls whether an extension-owned footer replaces
// the complete standard footer, including the keyed status row. The custom
// footer receives those statuses through FooterData and owns their rendering.
func (s *FooterComponent) SetSuppressedByExtFooter(v bool) {
	s.mu.Lock()
	s.suppressedByExtFooter = v
	s.mu.Unlock()
	s.Invalidate()
}

// Render returns the footer lines (normally 2 plus keyed statuses).
func (s *FooterComponent) Render(width int) []string {
	totals, routed, snap := s.footerSources()
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.suppressedByExtFooter {
		return nil
	}

	return renderFooter(s.footerDataLocked(totals, routed, snap), width)
}

// footerSources reads the usage totals and the routed model, which may take
// the session's locks, without holding s.mu.
func (s *FooterComponent) footerSources() (footerUsageTotals, *RoutedModelSelection, footerSnapshot) {
	s.mu.RLock()
	source, routedSource, roleSource := s.usageTotals, s.routedModel, s.activeRole
	s.mu.RUnlock()
	var totals footerUsageTotals
	if source != nil {
		totals = source()
	}
	var routed *RoutedModelSelection
	if routedSource != nil {
		routed = routedSource()
	}
	role, roleColor := "", ""
	if roleSource != nil {
		role, roleColor = roleSource()
	}
	cwd, gitBranch, providerCount, extensionStatuses, gitStatus := s.snapshot()
	return totals, routed, footerSnapshot{cwd: cwd, gitBranch: gitBranch, gitStatus: gitStatus, providerCount: providerCount, extensionStatuses: extensionStatuses, role: role, roleColor: roleColor}
}

// footerSnapshot is the footer data provider's state, read together without holding s.mu.
type footerSnapshot struct {
	cwd, gitBranch    string
	gitStatus         gitStatusCounts
	providerCount     int
	extensionStatuses map[string]string
	// role is the logical role of the current selection, or "" when it has none.
	role string
	// roleColor is that role's theme color token, as the model catalog paints it.
	roleColor string
}

// footerDataLocked snapshots the footer's inputs; the caller holds s.mu.
func (s *FooterComponent) footerDataLocked(totals footerUsageTotals, routed *RoutedModelSelection, snap footerSnapshot) footerData {
	return footerData{
		model:                  s.model,
		agentName:              s.agentName,
		sessionName:            s.name,
		cwd:                    snap.cwd,
		gitBranch:              snap.gitBranch,
		gitStatus:              snap.gitStatus,
		usage:                  totals,
		routed:                 routed,
		contextTokens:          s.contextTokens,
		contextUnknown:         s.contextUnknown,
		projectedContextWindow: s.projectedContextWindow,
		timings:                s.timings,
		working:                s.working,
		thinkingLevel:          s.thinkingLevel,
		autoCompactEnabled:     s.autoCompactEnabled,
		providerCount:          snap.providerCount,
		role:                   snap.role,
		roleColor:              snap.roleColor,
		usingSubscription:      s.usingSubscription,
		extensionStatuses:      snap.extensionStatuses,
	}
}

// footerData is the snapshot of all data needed to render the footer.
// Passed to renderFooter as a value so the free function can be tested.
type footerData struct {
	model                  *ai.Model
	agentName              string
	sessionName            string
	cwd                    string
	gitBranch              string
	gitStatus              gitStatusCounts
	usage                  footerUsageTotals
	routed                 *RoutedModelSelection
	contextTokens          int // last turn's total tokens for context% (not cumulative)
	contextUnknown         bool
	projectedContextWindow int
	timings                *agent.Recorder
	working                bool
	thinkingLevel          string
	autoCompactEnabled     bool
	// providerCount is the number of authenticated+reachable providers.
	// When >1, the model line shows "(provider) model" instead of just "model".
	// Mirrors upstream footer.ts:165-170 / footer-data-provider.ts.
	providerCount int
	// usingSubscription is true when the model's provider uses OAuth
	// subscription pricing (e.g. GitHub Copilot). Shows "(sub)" in cost.
	// Mirrors upstream footer.ts:128.
	usingSubscription bool
	// extensionStatuses: keyed status text set by extensions via setStatus.
	// Rendered as a third footer line (sorted by key). Mirrors upstream
	// footer.ts:205-215.
	extensionStatuses map[string]string
	// role is the logical role the current selection belongs to, or "" when the
	// session has none (a direct model selection, or a virtual model). It is shown
	// beside the model because Ctrl+P cycling and fallback can land on two roles
	// that resolve to the same physical model, which the name alone cannot
	// distinguish. Empty omits it, so a session without roles renders as before.
	role string
	// roleColor is the theme token the role is painted with. It comes from the role
	// metadata the model manager uses, so the same role reads the same hue in both
	// places; empty or unknown falls back to statusLineContext.
	roleColor string
}

// footerUsageTotals is upstream footer.ts's usageTotals over every session
// entry plus the cache-hit rate of the latest assistant message.
type footerUsageTotals struct {
	input, output, cacheRead, cacheWrite int
	cost                                 float64
	latestCacheHitRate                   *float64
}

// footerUsageParts renders footer.ts's token, cache-hit, and cost stats, each in
// the statusLine* token OMP's status line uses for it: spend counters in
// statusLineSpend, output in statusLineOutput, cost in statusLineCost. Kimi
// Coding and OAuth subscription logins show the cost even at zero, with
// "(sub)"; otherwise a zero total shows no cost segment.
func footerUsageParts(u footerUsageTotals, usingSubscription bool) []string {
	var parts []string
	spend := tui.ThemeColorStatusLineSpend
	if u.input != 0 {
		parts = append(parts, footerToken(spend, "↑"+formatTokens(u.input)))
	}
	if u.output != 0 {
		parts = append(parts, footerToken(tui.ThemeColorStatusLineOutput, "↓"+formatTokens(u.output)))
	}
	if u.cacheRead != 0 {
		parts = append(parts, footerToken(spend, "R"+formatTokens(u.cacheRead)))
	}
	if u.cacheWrite != 0 {
		parts = append(parts, footerToken(spend, "W"+formatTokens(u.cacheWrite)))
	}
	if (u.cacheRead > 0 || u.cacheWrite > 0) && u.latestCacheHitRate != nil {
		parts = append(parts, footerToken(spend, "CH"+jsstring.ToFixed(*u.latestCacheHitRate, 1)+"%"))
	}
	if (u.cost != 0 && !math.IsNaN(u.cost)) || usingSubscription {
		costStr := "$" + jsstring.ToFixed(u.cost, 3)
		if usingSubscription {
			costStr += " (sub)"
		}
		parts = append(parts, footerToken(tui.ThemeColorStatusLineCost, costStr))
	}
	return parts
}

// formatCwdForFooter abbreviates home and its descendants as "~" and
// "~<sep><relative path>", and leaves every other cwd, including a sibling
// that only shares home's prefix, as given. The caller passes upstream's home,
// HOME then USERPROFILE. Mirrors upstream footer.ts formatCwdForFooter.
func formatCwdForFooter(cwd, home string) string {
	if home == "" {
		return cwd
	}
	resolvedCwd, err := nodepath.Resolve(cwd)
	if err != nil {
		return cwd
	}
	resolvedHome, err := nodepath.Resolve(home)
	if err != nil {
		return cwd
	}
	relativeToHome, err := filepath.Rel(resolvedHome, resolvedCwd)
	if err != nil {
		return cwd
	}
	if relativeToHome == "." {
		return "~"
	}
	if relativeToHome == ".." || strings.HasPrefix(relativeToHome, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeToHome) {
		return cwd
	}
	return "~" + string(filepath.Separator) + relativeToHome
}

// footerToken is `theme.fg(token, text)` for the active theme, read when the
// line is drawn so a theme switch repaints the footer. An empty text stays
// empty: styling it would emit escape sequences for nothing.
func footerToken(token tui.ThemeColor, text string) string {
	if text == "" {
		return ""
	}
	return tui.ActiveTheme().Fg(string(token), text)
}

// footerSeparator is the ` · ` between two footer segments, in the theme's
// statusLineSep. It is its own color because a separator drawn in the next
// segment's color reads as part of that segment.
func footerSeparator() string { return footerToken(tui.ThemeColorStatusLineSep, " \u2022 ") }

// renderFooter produces the 2-line footer. Its segments come from upstream
// footer.ts, but the model shares line 1 with the project so that a narrow
// terminal still shows both on the row under the prompt:
//
//	Line 1: pwd (branch) • name                                  model • thinking
//	Line 2: ↑in ↓out [Rcache] [Wcache] $cost context%/window (auto)
//
// Every segment carries its own statusLine* token, as OMP's status line does:
// path, branch, session name, the token counters, cost, context window and the
// model each have a hue, so no part of the line depends on the terminal's default
// foreground.
func renderFooter(d footerData, width int) []string {
	if width <= 0 {
		width = 80
	}

	line1, line2 := footerLines(d, width)
	result := []string{line1, line2}

	// ── Line 3: extension statuses (optional) ──────────────────────────
	// Mirrors upstream footer.ts:205-215: sorted by key, space-separated,
	// sanitized (control chars → space), truncated to width.
	if status, ok := renderExtensionStatuses(d.extensionStatuses, width); ok {
		result = append(result, status)
	}

	return result
}

// footerLines lays out the two rows above the extension statuses.
//
// Line 1 carries the project on the left and the model on the right, the two facts
// a session is identified by, so a short terminal width shows both on the row under
// the prompt instead of letting one of them take a row of its own. Line 2 then
// holds the whole stats row. When the model cannot be shown beside even the
// project's leaf, the footer falls back to upstream's layout — the project alone on
// line 1, the model right-aligned on line 2 — because two rows still beat one row
// that lost a fact.
func footerLines(d footerData, width int) (string, string) {
	stats := footerStats(d)
	withProvider, plain := footerModelSide(d)
	path := footerCwd(d.cwd)
	branch, name := footerProjectDecorations(d)
	minPad := 2
	// Prefer the provider-qualified model; a narrower terminal keeps the model and
	// loses only the qualifier, which names where it comes from rather than what it
	// is (upstream footer.ts:167-173). The two forms are equal until a second
	// provider is available, and Compact then keeps the loop from trying the same
	// budget twice.
	for _, model := range slices.Compact([]string{withProvider, plain}) {
		modelWidth := widthx.VisibleWidth(model)
		project, ok := footerProject(path, branch, name, width-minPad-modelWidth)
		if !ok {
			continue
		}
		padding := strings.Repeat(" ", max(0, width-widthx.VisibleWidth(project)-modelWidth))
		return project + padding + model, widthx.TruncateToWidth(stats, width, "...", false)
	}
	// The model does not fit beside the project even with the full width, so the two
	// rows carry one fact each: the project on line 1 and the model on line 2 beside
	// the stats. Upstream's layout, reached only by terminals this narrow because a
	// shared row is what keeps both visible at phone widths.
	project, _ := footerProject(path, branch, name, width)
	if project == "" {
		// Widths too small for the compacted path still get its head: an empty row
		// would read as a rendering fault rather than as a truncated footer.
		project = widthx.TruncateToWidth(footerToken(tui.ThemeColorStatusLinePath, path), width, dim("..."), false)
	}
	return project, footerStatsWithModel(stats, withProvider, plain, width)
}

// footerProject renders line 1's project segment — the home-abbreviated working
// directory, the branch, and the session name — inside budget columns.
//
// Decorations are dropped before the path is: when the full path no longer fits,
// the session name goes first and then the branch, because the branch ties the row
// to the checkout while the name only names the conversation. The path itself is
// never dropped, only middle-elided, so the leaf that names the project survives
// any width the caller can give it.
//
// The bool reports whether the segment fit at all: false means budget cannot
// show even the path's leaf, and the caller must widen it by taking line 1 for the
// project alone.
func footerProject(path, branch, name string, budget int) (string, bool) {
	// The decorations are already styled, and VisibleWidth ignores the escapes, so
	// the suffix width is what it costs the path.
	//
	// The stages trade away the least important thing first: keep everything and
	// elide the middle, then drop the session name, then the branch, and only then
	// cut the leaf itself. The leaf is cut last because a cut name reads as a
	// different project, which is why the row surrenders its decorations first.
	// Empty decorations make the earlier stages repeat, and compactFooterPath answers
	// them identically, so the repetition costs one comparison each.
	for _, stage := range []struct {
		suffix       string
		allowLeafCut bool
	}{
		{branch + name, false},
		{branch, false},
		{"", false},
		{"", true},
	} {
		compacted, ok := compactFooterPath(path, budget-widthx.VisibleWidth(stage.suffix), stage.allowLeafCut)
		if !ok {
			continue
		}
		return footerToken(tui.ThemeColorStatusLinePath, compacted) + stage.suffix, true
	}
	return "", false
}

// footerProjectDecorations builds the styled suffix shared by every project-width
// attempt in one render. Keeping it outside footerProject avoids reapplying theme
// styles when the model fallback tries multiple budgets.
func footerProjectDecorations(d footerData) (branch, name string) {
	// The branch is statusLineContext, not statusLineGitClean/Dirty: this footer
	// resolves only the branch name, so coloring it clean or dirty would claim a
	// working-tree state it never read. OMP reads staged, modified and untracked
	// counts on its status line, and that is where those two colors belong.
	if d.gitBranch != "" {
		branchText := footerToken(tui.ThemeColorStatusLineContext, d.gitBranch)
		if d.gitStatus.unstaged > 0 {
			branchText += " " + footerToken(tui.ThemeColorStatusLineGitDirty, "*"+strconv.Itoa(d.gitStatus.unstaged))
		}
		if d.gitStatus.staged > 0 {
			branchText += " " + footerToken(tui.ThemeColorStatusLineStaged, "+"+strconv.Itoa(d.gitStatus.staged))
		}
		if d.gitStatus.untracked > 0 {
			branchText += " " + footerToken(tui.ThemeColorStatusLineUntracked, "?"+strconv.Itoa(d.gitStatus.untracked))
		}
		branch = footerToken(tui.ThemeColorStatusLineSep, " (") + branchText + footerToken(tui.ThemeColorStatusLineSep, ")")
	}
	if d.sessionName != "" {
		name = footerSeparator() + footerToken(tui.ThemeColorStatusLineModel, d.sessionName)
	}
	return branch, name
}

// compactFooterPath fits path into budget columns by eliding its middle. The first
// segment is the anchor a reader needs (the home marker, the root, or a Windows
// drive) and the last segments are the part that names the project, so the
// segments in between are what go. Candidates drop as few of them as possible
// first, so the result degrades from "~/…/a/b/c" through "~/…/c" to "~/c", and the
// shortest form that still names the project wins.
//
// allowLeafCut decides whether the last segment may be truncated once the anchor and
// the leaf no longer fit together. It is false for the first attempts, so a project
// name is only ever cut after the caller has given up the decorations that shared
// its row.
//
// It reports false when the path cannot be shown within budget on these terms; the
// caller then either widens the budget or allows the cut.
func compactFooterPath(path string, budget int, allowLeafCut bool) (string, bool) {
	if budget <= 0 {
		return "", false
	}
	if widthx.VisibleWidth(path) <= budget {
		return path, true
	}
	separator := string(filepath.Separator)
	segments := strings.Split(path, separator)
	last := len(segments) - 1
	if last == 0 {
		// A single segment, so there is no middle to elide: "~" on its own, or a
		// relative directory name.
		return cutLeaf(path, budget, allowLeafCut)
	}
	head, leaf := segments[0], segments[last]
	// The ellipsis marks the segments it replaces, so it only appears while one is
	// actually dropped: "~/…/c", never "~/…/" for a path that always had two parts.
	for drop := 1; drop <= last-1; drop++ {
		candidate := head + separator + "…" + separator + strings.Join(segments[1+drop:], separator)
		if widthx.VisibleWidth(candidate) <= budget {
			return candidate, true
		}
	}
	// The anchor and the leaf with nothing elided between them is shorter than the
	// narrowest elided form, and still says the project sits under the anchor.
	if candidate := head + separator + leaf; widthx.VisibleWidth(candidate) <= budget {
		return candidate, true
	}
	if widthx.VisibleWidth(leaf) <= budget {
		return leaf, true
	}
	return cutLeaf(leaf, budget, allowLeafCut)
}

// cutLeaf truncates the path's last segment to budget columns, or refuses when the
// caller has not given up its decorations yet.
func cutLeaf(leaf string, budget int, allow bool) (string, bool) {
	if !allow || budget <= 1 {
		return "", false
	}
	return widthx.TruncateToWidth(leaf, budget, "…", false), true
}

// footerStats renders line 2's left side: the token counters, the cost and the
// context window.
func footerStats(d footerData) string {
	parts := footerUsageParts(d.usage, d.usingSubscription)

	// Context usage: context%/window (auto). The percentage takes over the
	// segment's color as the window fills, because at that point the number is
	// the message and the window size is context.
	contextWindow, pct := footerContextUsage(d)
	autoTag := ""
	if d.autoCompactEnabled {
		autoTag = " (auto)"
	}
	display := jsstring.ToFixed(pct, 1) + "%/" + formatTokens(contextWindow) + autoTag
	if d.contextUnknown {
		display = fmt.Sprintf("?/%s%s", formatTokens(contextWindow), autoTag)
	}
	parts = append(parts, colorContextDisplay(pct, display))

	// Experimental features indicator. Upstream footer.ts:162-164 pushes a dim
	// "•" separator plus a bold warning "xp" badge onto the stats when
	// PI_EXPERIMENTAL=1. Off by default, so the idle footer is unchanged.
	if experimentalFeaturesEnabled() {
		parts = append(parts, dim("•")+" "+boldWarning("xp"))
	}

	return strings.Join(parts, footerSeparator())
}

// footerModelSide renders the model segment: the role and model name, the
// thinking level, and where a virtual model routed the latest request. It returns
// the preferred form, which names the provider while more than one is available,
// and the bare form for a terminal too narrow for the qualifier
// (footer.ts:165-174).
func footerModelSide(d footerData) (withProvider, plain string) {
	modelName, modelProvider, thinkingLevel := footerModel(d)
	side := footerSelection(d.role, d.roleColor, modelName)
	switch thinkingLevel {
	case "":
	case "off":
		side += footerSeparator() + footerToken(tui.ThemeColorStatusLineSep, "thinking "+thinkingLevel)
	default:
		side += footerSeparator() + footerToken(tui.ThemeColorStatusLineContext, thinkingLevel)
	}

	// A virtual model routes each request; show where the latest response went (footer.ts:237-243).
	if d.routed != nil && d.routed.Model != nil {
		side += footerToken(tui.ThemeColorStatusLineSep, " \u2192 ") +
			footerToken(tui.ThemeColorStatusLineContext, d.routed.Model.ID)
		if d.routed.ThinkingLevel != "" {
			side += footerSeparator() + footerToken(tui.ThemeColorStatusLineContext, string(d.routed.ThinkingLevel))
		}
	}

	if d.providerCount > 1 && modelProvider != "" {
		return footerToken(tui.ThemeColorStatusLineSep, "("+modelProvider+") ") + side, side
	}
	return side, side
}

// footerMinStatsWidth is the narrowest slice of the stats row worth keeping beside
// the model on the fallback row. It is the width of the in/out counters with their
// separator ("↑12k • ↓3.4k"); a narrower slice would be one counter and an
// ellipsis, which reports less than the model name does.
const footerMinStatsWidth = 14

// footerStatsWithModel is the narrow-terminal fallback row: the stats on the left
// and the model right-aligned, composed as upstream footer.ts:167-173 does. That
// composition keeps the stats and truncates the model, which is right for upstream
// because the model has a row of its own to be cut on. Here the model reached this
// row by failing to fit beside the project, so it is what the row is for: the stats
// are truncated to make room for it, and once they could not stay readable the
// model takes the row and the stats give up what is left of it.
func footerStatsWithModel(stats, withProvider, plain string, width int) string {
	minPad := 2
	for _, model := range slices.Compact([]string{withProvider, plain}) {
		modelWidth := widthx.VisibleWidth(model)
		room := width - minPad - modelWidth
		if room < footerMinStatsWidth {
			continue
		}
		truncated := widthx.TruncateToWidth(stats, room, "...", false)
		padding := strings.Repeat(" ", max(0, width-widthx.VisibleWidth(truncated)-modelWidth))
		return truncated + padding + model
	}
	// Right-align the model in what room is left, so the row still ends with the
	// model's own name rather than with its first characters.
	if modelWidth := widthx.VisibleWidth(plain); modelWidth < width {
		return strings.Repeat(" ", width-modelWidth) + plain
	}
	return widthx.TruncateToWidth(plain, width, "…", false)
}

// renderExtensionStatuses is footer.ts:205-215: the statuses sorted by key with localeCompare, each sanitized (sanitizeStatusText) and
// joined by spaces on one row, truncated to the width with a dim ellipsis. An empty sanitized text still takes its place.
func renderExtensionStatuses(statuses map[string]string, width int) (string, bool) {
	parts := footerExtensionStatuses(statuses)
	if len(parts) == 0 {
		return "", false
	}
	return widthx.TruncateToWidth(strings.Join(parts, " "), width, dim("..."), false), true
}

// footerExtensionStatuses returns the non-blank extension statuses in key
// order, each with its runs of white space collapsed, as footer.ts's
// sanitizeStatusText does.
func footerExtensionStatuses(statuses map[string]string) []string {
	if len(statuses) == 0 {
		return nil
	}
	keys := slices.Collect(maps.Keys(statuses))
	collator := localecompare.NewCollator()
	slices.SortStableFunc(keys, func(a, b string) int { return collator.CompareString(a, b) })
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = sanitizeStatusText(statuses[key])
	}
	return parts
}

// footerCwd is the footer's working directory with home abbreviated.
// Upstream's session cwd is always absolute. Without one, pig shows ".",
// which is not a path to abbreviate.
func footerCwd(cwd string) string {
	if cwd == "" {
		return "."
	}
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	return formatCwdForFooter(cwd, home)
}

// footerContextUsage returns the context window and the context's percentage
// of it. Upstream footer.ts uses session.getContextUsage(), which returns the
// last turn's token count (not cumulative), the actual current context window
// pressure.
func footerContextUsage(d footerData) (contextWindow int, pct float64) {
	// The window is getContextUsage's: the limits model's, which under a virtual selection is the physical model that answered last, then the selected model's (footer.ts:162, agent-session.ts:4139-4144 _limitsModel).
	limits := d.model
	if d.routed != nil && d.routed.Model != nil {
		limits = d.routed.Model
	}
	switch {
	case limits != nil && limits.Capabilities.ContextWindow > 0:
		contextWindow = limits.Capabilities.ContextWindow
	case d.model != nil:
		contextWindow = d.model.Capabilities.ContextWindow
	case d.projectedContextWindow > 0:
		contextWindow = d.projectedContextWindow
	}
	if contextWindow > 0 {
		pct = float64(d.contextTokens) / float64(contextWindow) * 100
	}
	return contextWindow, pct
}

// footerSelection renders the current selection as an optional role prefix plus the
// model name. The role comes first because it is what distinguishes two roles that
// share one physical model, which is exactly the state Ctrl+P cycling and fallback
// produce. A session with no role renders the bare model name, as upstream does.
//
// The role is painted with roleColor, the token the model catalog and manager use
// for that same role, so one role keeps one hue across the UI. A missing or
// undefined token falls back to statusLineContext, the hue this line already uses
// for secondary facts: it reads as a qualifier on the model rather than competing
// with statusLineModel.
func footerSelection(role, roleColor, modelName string) string {
	model := footerToken(tui.ThemeColorStatusLineModel, modelName)
	if role == "" {
		return model
	}
	return footerRoleToken(roleColor, "@"+role) + " " + model
}

// footerRoleToken paints the role with its catalog color. It reads
// tui.ModelOverlayTheme, not the active theme, because that is the palette the
// model manager and browser paint role rows with: the two are the same object for a
// light or explicit custom theme, but on the inherited stock dark/system themes the
// catalog uses OMP's Titanium colors while the transcript keeps PiG's stock ones, so
// the active theme would show every role a different hue than the manager.
//
// The guard is required, not defensive: Theme.Fg panics on an unknown token, and a
// role color can come from user-authored modelTags that settings accept as any
// string. HasFgToken asks that of the table Fg reads; GetResolvedThemeColors would
// not work, because it is empty for the catalog's synthesized Titanium projection
// even though that projection draws fine.
func footerRoleToken(color, text string) string {
	theme := tui.ModelOverlayTheme()
	if theme.HasFgToken(color) {
		return theme.Fg(color, text)
	}
	return footerToken(tui.ThemeColorStatusLineContext, text)
}

// footerModel returns the model's id, its provider's id, and the thinking
// level of a model that reasons ("off" when unset), or "" for one that does
// not. The upstream Agent supplies its "unknown" default model when no model
// is selected, so the footer still renders a stable model identity.
func footerModel(d footerData) (name, provider, thinkingLevel string) {
	if d.model == nil {
		return "unknown", "", ""
	}
	if d.model.Provider != nil {
		provider = d.model.Provider.ID()
	}
	if d.model.Capabilities.MaxThinking != "" {
		thinkingLevel = d.thinkingLevel
		if thinkingLevel == "" {
			thinkingLevel = "off"
		}
	}
	return d.model.ID, provider, thinkingLevel
}

// sanitizeStatusText is footer.ts sanitizeStatusText: carriage returns, newlines and tabs become spaces, runs of spaces collapse to
// one, and JavaScript's trim removes the ends. Other whitespace, such as a no-break space, stays.
func sanitizeStatusText(text string) string {
	text = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(text)
	var b strings.Builder
	previousSpace := false
	for _, r := range text {
		if r == ' ' && previousSpace {
			continue
		}
		previousSpace = r == ' '
		b.WriteRune(r)
	}
	return jsstring.Trim(b.String())
}

// colorContextDisplay wraps the full `pct%/window (auto)` display string in the
// color appropriate to the percent.
func colorContextDisplay(pct float64, display string) string {
	return applyContextColor(pct, display)
}

// applyContextColor colors the context display by OMP's four usage levels
// (chrome/context-thresholds.ts): error above 90%, thinkingHigh in the 70-90%
// "purple" band, warning above 50%, and statusLineContext below that. The level
// is the message, so the whole `pct%/window (auto)` token takes the color
// rather than only the number.
func applyContextColor(pct float64, body string) string {
	switch {
	case pct > 90:
		return footerToken(tui.ThemeColorError, body)
	case pct > 70:
		return footerToken(tui.ThemeColorThinkingHigh, body)
	case pct > 50:
		return footerToken(tui.ThemeColorWarning, body)
	default:
		return footerToken(tui.ThemeColorStatusLineContext, body)
	}
}

// FormatTokens is footer.ts formatTokens for presentations outside the interactive footer.
func FormatTokens(n int) string { return formatTokens(n) }

// formatTokens renders an int token count as "1.2k", "230", "8.4M", as footer.ts formatTokens does: toFixed(1) below ten thousand and ten million, Math.round (half up) elsewhere.
func formatTokens(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 10_000:
		return jsstring.ToFixed(float64(n)/1_000, 1) + "k"
	case n < 1_000_000:
		return strconv.Itoa(int(jsnumber.Round(float64(n)/1_000))) + "k" //portlint:allow numbers n is an int in [10_000,1_000_000) (or >=10_000_000), so the rounded quotient is finite and below 2^63: int() is exact (node differential over 39062 counts incl. rounding boundaries and 2^53)
	case n < 10_000_000:
		return jsstring.ToFixed(float64(n)/1_000_000, 1) + "M"
	default:
		return strconv.Itoa(int(jsnumber.Round(float64(n)/1_000_000))) + "M" //portlint:allow numbers n is an int in [10_000,1_000_000) (or >=10_000_000), so the rounded quotient is finite and below 2^63: int() is exact (node differential over 39062 counts incl. rounding boundaries and 2^53)
	}
}

// dim wraps text in the theme's dim foreground. Mirrors upstream footer.ts:
// theme.fg("dim", text), the token's resolved color (e.g.
// \x1b[38;2;102;102;102m for dark) and an fg-only reset. The system theme's
// dim token is faint text in the terminal's default color, closed with
// \x1b[22;39m (theme.ts fg of a dim token).
func dim(s string) string {
	return tui.ActiveTheme().Fg("dim", s)
}

// boldWarning renders text in bold with the theme's warning foreground,
// mirroring upstream footer.ts theme.bold(theme.fg("warning", text)): chalk.bold
// wraps \x1b[1m..\x1b[22m around theme.fg's <warning>text\x1b[39m, so the bytes
// are \x1b[1m<warning>text\x1b[39m\x1b[22m.
func boldWarning(s string) string {
	th := tui.ActiveTheme()
	return tui.ChalkBold(th.Warning + s + "\x1b[39m")
}

// stripANSI removes ANSI escape sequences (delegates to widthx.StripAnsi).
func stripANSI(s string) string { return widthx.StripAnsi(s) }

// resolveGitBranchFromPaths reads the bound HEAD, asking Git only for a reftable placeholder, as FooterDataProvider.resolveGitBranchSync does.
//
// Ports packages/coding-agent/src/core/footer-data-provider.ts (resolveGitBranchSync and resolveBranchWithGitSync).
func resolveGitBranchFromPaths(paths gitPaths) string {
	content, err := os.ReadFile(paths.headPath)
	if err != nil {
		return ""
	}
	branch, ok := strings.CutPrefix(jsTrim(string(content)), "ref: refs/heads/")
	if !ok {
		return "detached"
	}
	if branch == ".invalid" {
		if resolved := resolveBranchWithGit(context.Background(), paths.repoDir); resolved != "" {
			return resolved
		}
		return "detached"
	}
	return branch
}

// resolveBranchWithGit asks git for the current branch. It returns "" on a
// detached HEAD or when git is unavailable. Mirrors upstream
// resolveBranchWithGitSync; tests replace it to observe process spawns.
var resolveBranchWithGit = func(ctx context.Context, repoDir string) string {
	cmd := gitBranchCommand(ctx, repoDir)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return jsTrim(string(out))
}

func gitBranchCommand(ctx context.Context, repoDir string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", "--no-optional-locks", "symbolic-ref", "--quiet", "--short", "HEAD")
	cmd.Dir = repoDir
	// Upstream's spawnSync and execFile find git with libuv's search on
	// Windows, starting in repoDir.
	nodespawn.SetProgram(cmd)
	return cmd
}

// SetContextUsage stores the Session projection estimate outside the render path.
// Nil tokens indicate unknown usage after compaction until a valid response.
func (s *FooterComponent) SetContextUsage(tokens *int, contextWindow int) {
	s.mu.Lock()
	s.contextUnknown = tokens == nil && contextWindow > 0
	s.projectedContextWindow = contextWindow
	s.contextTokens = 0
	if tokens != nil {
		s.contextTokens = *tokens
	}
	s.mu.Unlock()
	s.Invalidate()
}

// RoutedModelSelection is the physical model and thinking level a virtual model selection currently resolves to (AgentSession.routedModel).
type RoutedModelSelection struct {
	Model         *ai.Model
	ThinkingLevel ai.ModelThinkingLevel
}
