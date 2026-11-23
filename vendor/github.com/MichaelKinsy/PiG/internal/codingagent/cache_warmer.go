package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/jsstring"
)

// Port of upstream core/cache-warmer.ts.

const (
	// maxWarmingAgeMs: streaming warming never continues past this long after
	// the real request that started it.
	maxWarmingAgeMs int64 = 60 * 60_000
	// maxIdleWarmingAgeMs: idle warming uses a shorter horizon because
	// continuation estimates become less reliable with age.
	maxIdleWarmingAgeMs int64 = 30 * 60_000
	// cacheWarmingMinimumExpectedSavings: a refresh is sent only when it is
	// expected to save at least this many dollars.
	cacheWarmingMinimumExpectedSavings = 0.05
	// idleContinuationProbability is the chance that a real request arrives
	// before the cache entry expires while the agent sits idle.
	idleContinuationProbability = 0.15
)

// GetCacheWarmingDelayMs refreshes at 90% of the TTL while preserving at least
// ten seconds of margin. The bool is false when the TTL is too short. The delay is a float64 like the JavaScript
// number: a lifetime from a models.json promptCache can exceed the int64 range.
func GetCacheWarmingDelayMs(ttlMs float64) (float64, bool) {
	if ttlMs <= 10_000 {
		return 0, false
	}
	return max(1, math.Floor(math.Min(ttlMs*0.9, ttlMs-10_000))), true
}

// GetPromptCacheTtlMs returns the model's prompt-cache lifetime for the request's retention. Explicit retention wins over the environment; none or a missing tier has no lifetime.
func GetPromptCacheTtlMs(model *ai.Model, options ai.StreamOptions) (float64, bool) {
	retention := string(options.CacheRetention)
	if retention == "" {
		retention = "short"
		if cacheWarmingEnvValue("PI_CACHE_RETENTION", options.Env) == "long" {
			retention = "long"
		}
	}
	if retention == "none" {
		return 0, false
	}
	seconds, ok := model.PromptCache[retention]
	if !ok {
		return 0, false
	}
	return seconds * 1000, true
}

// cacheWarmingEnvValue mirrors getProviderEnvValue: request env first, then
// the process environment.
func cacheWarmingEnvValue(name string, env ai.ProviderEnv) string {
	if value := env[name]; value != "" {
		return value
	}
	return os.Getenv(name)
}

// IsReplayable reports whether replaying the request with a one-token output
// cap leaves its cache entry untouched. Anthropic's budget-based thinking
// derives budget_tokens from max_tokens, and Anthropic keys the message cache
// on that budget, so only adaptive thinking replays safely.
func IsReplayable(model *ai.Model, options ai.StreamOptions) bool {
	reasoning := options.Thinking != ""
	if !reasoning || model.ProviderMeta.API != ai.APIAnthropicMessages {
		return true
	}
	compat := model.ProviderMeta.Compat
	return compat != nil && compat.ForceAdaptiveThinking != nil && *compat.ForceAdaptiveThinking
}

// lastPromptTokens is the prompt size of the most recent real request on the
// branch, as reported by the provider.
// warmerBranch is the branch whose newest assistant usage the warmer prices. A Session supplies its context branch, which holds the same newest
// assistant message without reading history the context no longer holds.
func warmerBranch(manager CacheWarmerSessionManager) []SessionEntry {
	if session, ok := manager.(interface{ GetContextBranch() []SessionEntry }); ok {
		return session.GetContextBranch()
	}
	return manager.GetBranch()
}

func lastPromptTokens(entries []SessionEntry) int {
	for _, entry := range slices.Backward(entries) {
		if entry.Base().Type != "message" {
			continue
		}
		// A decoded entry carries its role and usage: the session holds an appended message without its record, which a probe would encode.
		if typed, ok := asMessage(entry); ok {
			assistant := typed.Message.Assistant
			if assistant == nil {
				continue
			}
			var usage ai.Usage
			if assistant.Usage != nil {
				usage = *assistant.Usage
			}
			return usage.Input + usage.CacheRead + usage.CacheWrite
		}
		var probe struct {
			Message struct {
				Role  string   `json:"role"`
				Usage ai.Usage `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(entry.Raw(), &probe) != nil || probe.Message.Role != "assistant" {
			continue
		}
		usage := probe.Message.Usage
		return usage.Input + usage.CacheRead + usage.CacheWrite
	}
	return 0
}

func price(model *ai.Model, usage ai.Usage) float64 {
	return ai.CalculateCost(model, &usage).Total
}

// CacheWarmingAction is Pi's warm-or-stop decision, the one type the extension API carries (types.ts CacheWarmingDecisionEvent.action).
type CacheWarmingAction = extension.CacheWarmingAction

const (
	CacheWarmingActionWarm = extension.CacheWarmingActionWarm
	CacheWarmingActionStop = extension.CacheWarmingActionStop
)

// CacheWarmingDecision holds the inputs and outcome of one warm-or-stop
// decision, as shown by /session.
type CacheWarmingDecision struct {
	// Phase is "streaming" while the agent run that sent the request is still
	// active, then "idle".
	Phase string `json:"phase"`
	// WarmCost is the price of this refresh: a cache read of the prompt plus
	// one output token.
	WarmCost float64 `json:"warmCost"`
	// MissCost is the extra price of the next real request if the entry is lost.
	MissCost float64 `json:"missCost"`
	// ContinuationProbability estimates the chance that a real request
	// arrives before the entry expires.
	ContinuationProbability float64 `json:"continuationProbability"`
	// ExpectedSavings is ContinuationProbability*MissCost - WarmCost.
	ExpectedSavings float64 `json:"expectedSavings"`
	// EconomicsAvailable is false when the prompt size or prices are unknown.
	EconomicsAvailable bool `json:"economicsAvailable"`
	// Action is "warm" when ExpectedSavings is at least $0.05.
	Action CacheWarmingAction `json:"action"`
}

// CacheWarmingDecisionEvent is fired before each refresh with Pi's decision filled in; an extension may override Action. It is the event
// the extension API delivers to cache_warming_decision handlers.
type CacheWarmingDecisionEvent = extension.CacheWarmingDecisionEvent

// CacheWarmingStatus is the warmer state /session reports.
type CacheWarmingStatus struct {
	// State is "inactive", "scheduled" (a refresh timer is armed), or
	// "refreshing" (a warm request is in flight).
	State string `json:"state"`
	// Reason says why nothing is scheduled.
	Reason string `json:"reason,omitempty"`
	// NextWarmAt is the Unix millisecond time of the next decision, or 0.
	NextWarmAt int64 `json:"nextWarmAt,omitempty"`
	// Decision is the pending decision, or the decision that stopped warming.
	Decision *CacheWarmingDecision `json:"decision,omitempty"`
	// ExtensionOverride is true when an extension changed Decision.Action.
	ExtensionOverride bool `json:"extensionOverride,omitempty"`
}

// CacheWarmRequest is the request whose prompt cache entry should be kept
// warm, exactly as it was sent.
type CacheWarmRequest struct {
	Model   *ai.Model
	Context ai.TranscriptContext
	Options ai.StreamOptions
	// IdleEligible, when set, reports whether the session's live context is
	// still exactly the request's context plus the successful response that
	// answered it. Idle compaction declines an offer once it reports false:
	// anything else (custom messages, bash results, context edits, a failed
	// or extra response) is not covered by the summary the offer would send.
	IdleEligible func() bool
}

// IdleOfferResult is the session's answer to an idle-compaction deadline
// offer. A busy answer leaves the offer pending; the warmer re-offers it when
// the session next becomes idle. Claimed and declined latch the offer.
type IdleOfferResult int

const (
	// IdleOfferBusy means the session was still settling or held pending
	// work: retry the same offer when it next goes idle.
	IdleOfferBusy IdleOfferResult = iota
	// IdleOfferClaimed means the session ran idle compaction for the offer.
	IdleOfferClaimed
	// IdleOfferDeclined means the offer no longer matches the session; it is
	// not offered again.
	IdleOfferDeclined
)

// CacheWarmerSessionManager is the Session surface the warmer uses. Mirrors
// Pick<SessionManager, "appendUsage" | "getBranch">.
type CacheWarmerSessionManager interface {
	AppendUsage(kind, provider, model string, usage ai.Usage, note string) (UsageEntry, error)
	GetBranch(fromID ...string) []SessionEntry
}

// CacheWarmingDecide lets an extension override a decision. An error falls
// back to Pi's decision.
type CacheWarmingDecide func(context.Context, CacheWarmingDecisionEvent) (CacheWarmingAction, error)

// IdleCompactionPolicy configures the idle-compaction deadline the warmer
// offers the session at a cache boundary.
type IdleCompactionPolicy struct {
	// Enabled gates the offer entirely (compaction.enabled and
	// compaction.idleEnabled on the session side).
	Enabled bool
	// DelaySeconds is the settled-idle wait used when the request's model
	// carries no promptCache lifetime. It is a provider-neutral fallback
	// (compaction.idleDelaySeconds), not a guessed cache TTL.
	DelaySeconds float64
}

type cacheWarmingRun struct {
	CacheWarmRequest
	// isCurrent is false once the session's model or messages no longer
	// match the request.
	isCurrent func() bool
	ttlMs     float64
	delayMs   float64
	// warmable is false when the request cannot be replayed (no cache
	// lifetime, replay-unsafe thinking, or warming disabled): the run then
	// exists only to carry its idle-compaction deadline.
	warmable bool
	// idleArmed marks a run that may offer its deadline to idle compaction.
	// idleTried latches once the offer is made, claimed or declined: at most
	// one idle compaction attempt per captured request.
	idleArmed bool
	idleTried bool
	// offering serializes offer callbacks: a deadline timer and an idle
	// notification never run two offers for one run.
	offering bool
	// idleDeadlineAt is the offer's absolute deadline in Unix milliseconds.
	// A run with a promptCache lifetime anchors it to the request start; a
	// non-warmable run without a lifetime anchors it at settle, using the
	// policy delay current then. Zero means the settle anchor is pending.
	idleDeadlineAt int64
	// refreshDeadlineAt is the latest safe time to send this refresh,
	// leaving half the original expiry margin.
	refreshDeadlineAt int64
	startedAt         int64
	ctx               context.Context
	cancel            context.CancelFunc
	phase             string
	nextWarmAt        int64
	// extensionOverride is set while a refresh that an extension forced is
	// in flight.
	extensionOverride bool
	// timer is nil while a refresh or the idle-compaction offer is running.
	timer *time.Timer
}

// CacheWarmer keeps one prompt cache entry alive by re-sending its request
// with a one-token output cap before the entry expires. Start replaces any
// previous run; warm requests never extend the fixed safety windows.
//
// Upstream's unawaited setTimeout refresh maps to an owned task: a timer whose
// callback the warmer tracks, cancelled through the run's context and drained
// by Wait.
type CacheWarmer struct {
	mu             sync.Mutex
	run            *cacheWarmingRun
	inactive       CacheWarmingStatus
	closed         bool
	stream         agent.StreamFn
	sessionManager CacheWarmerSessionManager
	getMode        func() CacheWarmingMode
	decide         CacheWarmingDecide
	onWarmed       func(context.Context, UsageEntry)
	// idleCompaction, when set, is offered each armed run's deadline once.
	// It runs on a goroutine tracked by tasks and must stop once the run's
	// context ends. IdleOfferBusy leaves the offer pending for the next idle
	// notification; the other answers latch it.
	idleCompaction func(ctx context.Context, request *CacheWarmRequest) IdleOfferResult
	// idlePolicy is re-read for every arming and offer, so a toggled or
	// retimed setting applies without reinstalling the hook.
	idlePolicy func() IdleCompactionPolicy
	tasks          sync.WaitGroup
	// lifetime is cancelled by Close. publishMu is held while onWarmed runs,
	// so Close can wait out a report that began before it.
	lifetime  context.Context
	dispose   context.CancelFunc
	publishMu sync.Mutex
}

// NewCacheWarmer creates a warmer. A nil decide keeps Pi's own decision.
func NewCacheWarmer(stream agent.StreamFn, sessionManager CacheWarmerSessionManager, getMode func() CacheWarmingMode, decide CacheWarmingDecide) *CacheWarmer {
	if decide == nil {
		decide = func(_ context.Context, event CacheWarmingDecisionEvent) (CacheWarmingAction, error) {
			return event.Action, nil
		}
	}
	lifetime, dispose := context.WithCancel(context.Background())
	return &CacheWarmer{
		stream:         stream,
		sessionManager: sessionManager,
		getMode:        getMode,
		decide:         decide,
		inactive:       inactiveCacheWarming("waiting for first request"),
		lifetime:       lifetime,
		dispose:        dispose,
	}
}

func inactiveCacheWarming(reason string) CacheWarmingStatus {
	return CacheWarmingStatus{State: "inactive", Reason: reason}
}

func cacheWarmingNowMs() int64 { return time.Now().UnixMilli() }

// SetOnWarmed sets the callback that receives the persisted usage entry after
// each successful refresh. Its context is cancelled when Close starts; a
// callback blocked on delivery must return once it is, because Close waits for
// it. The callback must not call Close.
func (w *CacheWarmer) SetOnWarmed(onWarmed func(context.Context, UsageEntry)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.onWarmed = onWarmed
	}
}

// SetIdleCompaction installs the session's idle-compaction hook and policy
// getter. The callback fires at an armed run's deadline instead of (or
// without) a warm refresh, before Pi's warm-or-stop evaluation can end the
// run. It is offered once per captured request.
func (w *CacheWarmer) SetIdleCompaction(cb func(ctx context.Context, request *CacheWarmRequest) IdleOfferResult, policy func() IdleCompactionPolicy) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.idleCompaction = cb
		w.idlePolicy = policy
	}
}

func (w *CacheWarmer) idlePolicyEnabled() bool {
	return w.idlePolicy != nil && w.idlePolicy().Enabled
}

func (w *CacheWarmer) idlePolicyDelayMs() float64 {
	if w.idlePolicy == nil {
		return 0
	}
	return w.idlePolicy().DelaySeconds * 1000
}

// Status reports the current warming state and the policy inputs that
// produced it.
func (w *CacheWarmer) Status() CacheWarmingStatus {
	if w.getMode() == "off" {
		return inactiveCacheWarming("cache warming disabled")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	run := w.run
	if run == nil {
		return w.inactive
	}
	if !run.isCurrent() {
		return inactiveCacheWarming("conversation context changed")
	}
	decision := w.evaluate(run)
	refreshing := run.timer == nil && run.warmable
	if !run.warmable {
		// A deadline-only run schedules no refresh; it reports its pending
		// offer, or inactivity once the offer was consumed.
		if run.timer != nil || run.offering || w.idleOfferPendingLocked(run) {
			return CacheWarmingStatus{State: "scheduled", NextWarmAt: run.nextWarmAt}
		}
		return w.inactive
	}
	if !decision.EconomicsAvailable && !refreshing {
		return inactiveCacheWarming("cache economics unavailable")
	}
	state := "scheduled"
	if refreshing {
		state = "refreshing"
	}
	return CacheWarmingStatus{State: state, NextWarmAt: run.nextWarmAt, Decision: &decision, ExtensionOverride: run.extensionOverride}
}

// Start keeps the prompt cache entry written by request warm while isCurrent
// holds. A request that cannot be replayed still starts a run when an idle
// compaction deadline is armed, so the session can compact at the boundary.
func (w *CacheWarmer) Start(request CacheWarmRequest, isCurrent func() bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.clearRunLocked()
	startedAt := cacheWarmingNowMs()
	ttlMs, hasLifetime := GetPromptCacheTtlMs(request.Model, request.Options)
	var ttlDelayMs float64
	if hasLifetime {
		ttlDelayMs, _ = GetCacheWarmingDelayMs(ttlMs)
	}
	// A known cache deadline anchors the offer to the request, so a run that
	// outlives it still offers. Without a lifetime the offer anchors at
	// settle: the delay is a provider-neutral fallback, not a guessed TTL.
	var idleDeadlineAt int64
	if ttlDelayMs > 0 {
		idleDeadlineAt = saturateToMillis(float64(startedAt) + ttlDelayMs)
	}
	idleArmed := w.idleCompaction != nil && w.idlePolicyEnabled() && (idleDeadlineAt > 0 || w.idlePolicyDelayMs() > 0)
	ctx, cancel := context.WithCancel(context.Background())
	run := &cacheWarmingRun{
		CacheWarmRequest: request,
		isCurrent:        isCurrent,
		ttlMs:            ttlMs,
		startedAt:        startedAt,
		ctx:              ctx,
		cancel:           cancel,
		phase:            "streaming",
		idleArmed:        idleArmed,
		idleDeadlineAt:   idleDeadlineAt,
	}
	if reason := w.startStopReason(request); reason != "" {
		if !idleArmed {
			cancel()
			w.stopLocked(reason, nil, false)
			return
		}
		// Not replayable: the run carries only its idle-compaction deadline,
		// which OnAgentSettled anchors to the moment the session goes idle.
		w.run = run
		return
	}
	run.delayMs = ttlDelayMs
	run.warmable = true
	w.run = run
	w.scheduleLocked(run)
}

// startStopReason says why a request cannot be warmed, or "" when it can.
func (w *CacheWarmer) startStopReason(request CacheWarmRequest) string {
	if w.getMode() == "off" {
		return "cache warming disabled"
	}
	if !IsReplayable(request.Model, request.Options) {
		return "request cannot be replayed safely"
	}
	ttlMs, ok := GetPromptCacheTtlMs(request.Model, request.Options)
	if !ok {
		return "cache lifetime unavailable"
	}
	if _, ok := GetCacheWarmingDelayMs(ttlMs); !ok {
		return "cache lifetime unavailable"
	}
	return ""
}

// OnAgentSettled moves the run to its idle phase, or stops it in streaming
// mode. A run whose idle-compaction deadline is still pending survives the
// settle in any mode: its offer replaces the work streaming mode would skip.
func (w *CacheWarmer) OnAgentSettled() {
	w.mu.Lock()
	defer w.mu.Unlock()
	run := w.run
	if run == nil {
		return
	}
	run.phase = "idle"
	if !run.warmable {
		w.scheduleIdleDeadlineLocked(run)
		return
	}
	if w.getMode() == "streaming" {
		if w.idleOfferPendingLocked(run) {
			return
		}
		w.stopLocked("agent run settled", nil, false)
		return
	}
	deadline := saturateToMillis(float64(run.startedAt) + float64(maxIdleWarmingAgeMs))
	if run.nextWarmAt > deadline || cacheWarmingNowMs() >= deadline {
		w.degradeToIdleDeadlineLocked(run, "30-minute idle safety limit reached")
	}
}

// OnIdle reports that the session just became idle. An armed run whose
// deadline passed, or whose last offer found the session busy, is offered
// again; anything else is left alone. The offer runs on a tracked task so
// the session's idle path never waits for compaction work.
func (w *CacheWarmer) OnIdle() {
	w.mu.Lock()
	run := w.run
	if run == nil || !w.offerIdleDeadlineLocked(run) {
		w.mu.Unlock()
		return
	}
	w.tasks.Add(1)
	w.mu.Unlock()
	go func() {
		defer w.tasks.Done()
		defer RecoverUncaught()
		w.runIdleOffer(run)
	}()
}

// offerIdleDeadlineLocked consumes the run's timer and marks its offer in
// flight when the deadline is due and no offer is running. w.mu must be held.
func (w *CacheWarmer) offerIdleDeadlineLocked(run *cacheWarmingRun) bool {
	if w.run != run || run.phase != "idle" || run.offering || !w.idleOfferPendingLocked(run) || cacheWarmingNowMs() < run.idleDeadlineAt {
		return false
	}
	if run.timer != nil && run.timer.Stop() {
		// The stopped AfterFunc cannot run, so its task accounting ends here.
		w.tasks.Done()
	}
	run.timer = nil
	run.offering = true
	return true
}

// runIdleOffer delivers a due deadline to the session hook. A busy answer
// leaves the run pending for the next idle signal; claimed and declined
// latch idleTried so the offer is made once per captured request. It runs on
// a tasks-tracked goroutine, so the post-offer refresh work may run inline.
func (w *CacheWarmer) runIdleOffer(run *cacheWarmingRun) {
	w.mu.Lock()
	cb := w.idleCompaction
	w.mu.Unlock()
	result := IdleOfferDeclined
	if cb != nil {
		result = cb(run.ctx, &run.CacheWarmRequest)
	}
	w.mu.Lock()
	run.offering = false
	if w.run != run {
		w.mu.Unlock()
		return
	}
	switch result {
	case IdleOfferClaimed:
		run.idleTried = true
		w.rescheduleAfterIdleLocked(run)
		w.mu.Unlock()
	case IdleOfferBusy:
		// Keep the entry warm while the session finishes settling; the offer
		// stays pending for the next idle signal.
		if run.warmable {
			w.scheduleLocked(run)
		}
		w.mu.Unlock()
	default: // IdleOfferDeclined
		run.idleTried = true
		if !run.warmable {
			w.stopLocked("inactive", nil, false)
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()
		// Declined: the normal warm-or-stop evaluation runs instead.
		w.refresh(run)
	}
}

// rescheduleAfterIdleLocked keeps the cache entry warm past a claimed idle
// compaction when the mode would still refresh it; otherwise the run ends.
// w.mu must be held.
func (w *CacheWarmer) rescheduleAfterIdleLocked(run *cacheWarmingRun) {
	if run.warmable && w.getMode() == "idle" {
		w.scheduleLocked(run)
		return
	}
	w.stopLocked("inactive", nil, false)
}

// idleOfferPendingLocked reports whether the run can still be offered to idle
// compaction. w.mu must be held.
func (w *CacheWarmer) idleOfferPendingLocked(run *cacheWarmingRun) bool {
	return run.idleArmed && !run.idleTried && w.idleCompaction != nil && w.idlePolicyEnabled()
}

// scheduleIdleDeadlineLocked arms the run's offer deadline. A deadline-less
// run anchors at this settle with the current policy delay; a run that
// already passed its absolute deadline stays pending for the idle signal
// that follows this settle instead of offering inside settling work.
// w.mu must be held.
func (w *CacheWarmer) scheduleIdleDeadlineLocked(run *cacheWarmingRun) {
	if run.timer != nil || run.offering {
		return
	}
	now := cacheWarmingNowMs()
	if run.idleDeadlineAt == 0 {
		run.idleDeadlineAt = saturateToMillis(float64(now) + w.idlePolicyDelayMs())
	}
	run.nextWarmAt = run.idleDeadlineAt
	if now >= run.idleDeadlineAt {
		return
	}
	w.tasks.Add(1)
	run.timer = time.AfterFunc(clampMillisDuration(run.idleDeadlineAt-now), func() {
		defer w.tasks.Done()
		defer RecoverUncaught()
		w.onTimer(run)
	})
}

// degradeToIdleDeadlineLocked ends a run's refresh work while preserving its
// pending idle-compaction offer: the run continues as deadline-only, the
// warming cap no longer applying to the one-shot compaction offer. A run
// without a pending offer stops for the given reason. w.mu must be held.
func (w *CacheWarmer) degradeToIdleDeadlineLocked(run *cacheWarmingRun, reason string) {
	if !w.idleOfferPendingLocked(run) {
		w.stopLocked(reason, nil, false)
		return
	}
	run.warmable = false
	w.scheduleIdleDeadlineLocked(run)
}

// saturateToMillis clamps a computed millisecond time to the int64 range, so
// a promptCache lifetime that overflows it schedules for the far future
// instead of wrapping to a past or undefined value.
func saturateToMillis(value float64) int64 {
	switch {
	case math.IsNaN(value):
		return 0
	case value >= math.MaxInt64:
		return math.MaxInt64
	case value <= math.MinInt64:
		return math.MinInt64
	default:
		return int64(value)
	}
}

// clampMillisDuration converts a millisecond distance to a positive
// time.Duration without overflowing int64 nanoseconds.
func clampMillisDuration(ms int64) time.Duration {
	const maxMillis = int64(math.MaxInt64) / int64(time.Millisecond)
	if ms <= 0 {
		return time.Millisecond
	}
	if ms > maxMillis {
		ms = maxMillis
	}
	return time.Duration(ms) * time.Millisecond
}

// OnModeChanged reconciles an active run after the persisted mode changes.
func (w *CacheWarmer) OnModeChanged() {
	w.mu.Lock()
	defer w.mu.Unlock()
	run := w.run
	if run == nil {
		return
	}
	if reason := w.modeStopReason(run); reason != "" {
		w.stopLocked(reason, nil, false)
	}
}

// Cancel stops warming. Mirrors upstream cancel().
func (w *CacheWarmer) Cancel() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopLocked("inactive", nil, false)
}

// Close cancels warming for good, as upstream dispose does: later Start calls
// are ignored and, once Close returns, no refresh reports to OnWarmed, even
// one still persisting its usage entry. Close waits for a report already in
// progress, after cancelling that report's context.
func (w *CacheWarmer) Close() {
	w.mu.Lock()
	w.closed = true
	w.onWarmed = nil
	w.stopLocked("inactive", nil, false)
	w.dispose()
	w.mu.Unlock()
	// An empty critical section waits out an in-progress report.
	w.publishMu.Lock()
	//lint:ignore SA2001 the empty critical section is the wait.
	w.publishMu.Unlock() //nolint:staticcheck // SA2001: see above.
}

// Wait blocks until every scheduled or in-flight refresh has finished, then releases the closed warmer's provider and Session references. Call it after Close.
func (w *CacheWarmer) Wait() {
	w.tasks.Wait()
	w.mu.Lock()
	w.sessionManager = nil
	w.stream = nil
	w.decide = nil
	w.mu.Unlock()
}

func (w *CacheWarmer) clearRunLocked() {
	run := w.run
	if run == nil {
		return
	}
	w.run = nil
	if run.timer != nil && run.timer.Stop() {
		// A stopped AfterFunc may remain in the timer heap. Its callback cannot run, so release the captured request and conversation immediately.
		run.CacheWarmRequest = CacheWarmRequest{}
		run.isCurrent = nil
		w.tasks.Done()
	}
	run.cancel()
}

func (w *CacheWarmer) stopLocked(reason string, decision *CacheWarmingDecision, extensionOverride bool) {
	w.clearRunLocked()
	w.inactive = CacheWarmingStatus{State: "inactive", Reason: reason, Decision: decision, ExtensionOverride: extensionOverride}
}

func (w *CacheWarmer) scheduleLocked(run *cacheWarmingRun) {
	run.extensionOverride = false
	now := cacheWarmingNowMs()
	next := float64(now) + run.delayMs
	limit, reason := maxWarmingAgeMs, "one-hour safety limit reached"
	if run.phase == "idle" {
		limit, reason = maxIdleWarmingAgeMs, "30-minute idle safety limit reached"
	}
	if deadline := saturateToMillis(float64(run.startedAt) + float64(limit)); next > float64(deadline) || now >= deadline {
		w.degradeToIdleDeadlineLocked(run, reason)
		return
	}
	run.nextWarmAt = saturateToMillis(next)
	// A timer can run late after sleep or a stalled process. Keep half of the
	// planned pre-expiry margin for that delay and request dispatch; a late
	// refresh is likely a full-price cache write, not a cache warm.
	run.refreshDeadlineAt = saturateToMillis(float64(run.nextWarmAt) + math.Floor((run.ttlMs-run.delayMs)/2))
	w.tasks.Add(1)
	run.timer = time.AfterFunc(clampMillisDuration(run.nextWarmAt-now), func() {
		defer w.tasks.Done()
		// pig additive (D102): a panic outside the request upstream catches reaches the interactive uncaught handler, as
		// upstream's floating refresh promise reaches uncaughtException.
		defer RecoverUncaught()
		w.onTimer(run)
	})
}

// onTimer is the deadline callback for warmable and deadline-only runs. An
// armed run offers its deadline to idle compaction first: the session claims
// it (compaction refreshes the cache entry itself, so the run reschedules
// while warming is still wanted), reports itself busy and keeps the offer
// pending for the next idle signal, or declines and the normal warm-or-stop
// evaluation runs instead.
func (w *CacheWarmer) onTimer(run *cacheWarmingRun) {
	w.mu.Lock()
	if w.offerIdleDeadlineLocked(run) {
		w.mu.Unlock()
		w.runIdleOffer(run)
		return
	}
	warmable := w.run == run && run.warmable
	pendingOffer := w.run == run && (w.idleOfferPendingLocked(run) || run.offering)
	w.mu.Unlock()
	if warmable {
		w.refresh(run)
		return
	}
	if !pendingOffer {
		w.mu.Lock()
		if w.run == run && !run.warmable && !w.idleOfferPendingLocked(run) && !run.offering {
			w.stopLocked("inactive", nil, false)
		}
		w.mu.Unlock()
	}
}

func (w *CacheWarmer) refresh(run *cacheWarmingRun) {
	decision, ok := w.beginRefresh(run)
	if !ok {
		return
	}
	action := decision.Action
	if decided, err := w.decideCaught(run.ctx, CacheWarmingDecisionEvent{
		Type:                    "cache_warming_decision",
		WarmCost:                decision.WarmCost,
		MissCost:                decision.MissCost,
		ContinuationProbability: decision.ContinuationProbability,
		Action:                  action,
	}); err == nil {
		action = decided
	}
	extensionOverride, warm := w.applyDecision(run, decision, action)
	if !warm || !w.sendRefresh(run, extensionOverride) {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.run == run {
		w.scheduleLocked(run)
	}
}

// beginRefresh marks the run as refreshing and evaluates Pi's decision.
func (w *CacheWarmer) beginRefresh(run *cacheWarmingRun) (CacheWarmingDecision, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	run.timer = nil
	if !w.validateRunLocked(run) || w.refreshDeadlineMissedLocked(run) {
		return CacheWarmingDecision{}, false
	}
	return w.evaluate(run), true
}

// applyDecision rechecks the run after the extension decision and stops it
// when the final action is "stop".
func (w *CacheWarmer) applyDecision(run *cacheWarmingRun, decision CacheWarmingDecision, action CacheWarmingAction) (extensionOverride, warm bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.validateRunLocked(run) || w.refreshDeadlineMissedLocked(run) {
		return false, false
	}
	extensionOverride = action != decision.Action
	if action == CacheWarmingActionStop {
		reason := "cache economics unavailable"
		switch {
		case extensionOverride:
			reason = "stopped by extension"
		case decision.EconomicsAvailable:
			reason = "expected savings below threshold"
		}
		// An economic stop ends refreshes, not a pending idle-compaction offer.
		w.degradeToIdleDeadlineLocked(run, reason)
		return extensionOverride, false
	}
	run.extensionOverride = extensionOverride
	return extensionOverride, true
}

// decideCaught asks the extensions for the action. A panic counts as the extension failure upstream catches, so Pi's own
// decision stands.
func (w *CacheWarmer) decideCaught(ctx context.Context, event CacheWarmingDecisionEvent) (action CacheWarmingAction, err error) {
	defer func() {
		// upstream: packages/coding-agent/src/core/cache-warmer.ts:refresh catches every decide failure ("Extension failures fall back to pi's own decision").
		if value := recover(); value != nil {
			debugLog("cache warming decision panicked: %v\n%s", value, debug.Stack())
			err = fmt.Errorf("cache warming decision panicked: %v", value)
		}
	}()
	return w.decide(ctx, event)
}

// sendRefresh replays the request with a one-token output cap and no provider
// retries, and records its usage. It reports whether the run should be
// rescheduled. Warming is best-effort: a failed request only skips the usage,
// and so does a panic anywhere upstream's try block covers (the request, its
// result, the run check, the usage write and the onWarmed callback).
func (w *CacheWarmer) sendRefresh(run *cacheWarmingRun, extensionOverride bool) (reschedule bool) {
	defer func() {
		// upstream: packages/coding-agent/src/core/cache-warmer.ts:refresh catches everything the refresh request throws ("Cache warming is best-effort and must not affect the active agent run").
		if value := recover(); value != nil {
			debugLog("cache warming refresh panicked: %v\n%s", value, debug.Stack())
			reschedule = true
		}
	}()
	options := run.Options
	options.MaxTokens = 1
	stream, err := w.stream(ai.WithProviderMaxRetries(run.ctx, 0), run.Model, run.Context, options)
	if err != nil {
		return true
	}
	message := stream.Result()
	if !w.validateRun(run) {
		return false
	}
	if message == nil || message.StopReason == ai.StopReasonError || message.StopReason == ai.StopReasonAborted {
		return true
	}
	model := message.ResponseModel
	if model == "" {
		model = message.Model
	}
	note := ""
	if extensionOverride {
		note = "extension override"
	}
	entry, err := w.sessionManager.AppendUsage("cache_warm", message.Provider, model, message.Usage, note)
	if err == nil {
		w.publishWarmed(entry)
	}
	return true
}

// publishWarmed reports a persisted entry unless Close has started.
// Persisting can block, and Close does not wait for it, so the callback is
// read only after the entry is written, under publishMu, which Close takes
// after clearing it.
func (w *CacheWarmer) publishWarmed(entry UsageEntry) {
	w.publishMu.Lock()
	defer w.publishMu.Unlock()
	w.mu.Lock()
	onWarmed := w.onWarmed
	w.mu.Unlock()
	if onWarmed != nil {
		onWarmed(w.lifetime, entry)
	}
}

func (w *CacheWarmer) refreshDeadlineMissedLocked(run *cacheWarmingRun) bool {
	if cacheWarmingNowMs() <= run.refreshDeadlineAt {
		return false
	}
	w.degradeToIdleDeadlineLocked(run, "cache refresh deadline missed")
	return true
}

func (w *CacheWarmer) validateRun(run *cacheWarmingRun) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.validateRunLocked(run)
}

func (w *CacheWarmer) validateRunLocked(run *cacheWarmingRun) bool {
	if w.run != run {
		return false
	}
	reason := w.modeStopReason(run)
	if reason == "" && !run.isCurrent() {
		reason = "conversation context changed"
	}
	if reason == "" {
		return true
	}
	w.stopLocked(reason, nil, false)
	return false
}

func (w *CacheWarmer) modeStopReason(run *cacheWarmingRun) string {
	// An armed run waiting to offer its idle-compaction deadline survives
	// modes that no longer pay for refreshes.
	if w.idleOfferPendingLocked(run) {
		return ""
	}
	switch mode := w.getMode(); {
	case mode == "off":
		return "cache warming disabled"
	case mode == "streaming" && run.phase == "idle":
		return "agent run settled"
	}
	return ""
}

func (w *CacheWarmer) evaluate(run *cacheWarmingRun) CacheWarmingDecision {
	model := run.Model
	promptTokens := lastPromptTokens(warmerBranch(w.sessionManager))
	cacheHitCost := price(model, ai.Usage{CacheRead: promptTokens})
	missUsage := ai.Usage{Input: promptTokens}
	if model.Capabilities.CacheWriteCostPer1M > 0 {
		missUsage = ai.Usage{CacheWrite: promptTokens}
	}
	cacheMissCost := price(model, missUsage)
	warmCost := price(model, ai.Usage{CacheRead: promptTokens, Output: 1})
	missCost := math.Max(0, cacheMissCost-cacheHitCost)
	continuationProbability := 1.0
	if run.phase == "idle" {
		continuationProbability = idleContinuationProbability
	}
	expectedSavings := float64(continuationProbability*missCost) - warmCost
	action := CacheWarmingActionStop
	if expectedSavings >= cacheWarmingMinimumExpectedSavings {
		action = CacheWarmingActionWarm
	}
	return CacheWarmingDecision{
		Phase:                   run.phase,
		WarmCost:                warmCost,
		MissCost:                missCost,
		ContinuationProbability: continuationProbability,
		ExpectedSavings:         expectedSavings,
		EconomicsAvailable:      promptTokens > 0 && (cacheHitCost > 0 || cacheMissCost > 0),
		Action:                  action,
	}
}

func formatDollars(value float64) string {
	if value < 0 {
		return "-$" + jsstring.ToFixed(math.Abs(value), 3)
	}
	return "$" + jsstring.ToFixed(value, 3)
}

func formatCacheWarmingEconomics(decision CacheWarmingDecision) string {
	if !decision.EconomicsAvailable {
		return "cache economics unavailable"
	}
	probability := int(math.Round(decision.ContinuationProbability * 100))
	probabilityText := fmt.Sprintf("%d%% continuation probability", probability)
	if decision.Phase == "streaming" {
		probabilityText += " while agent is running"
	}
	comparison := "<"
	if decision.Action == CacheWarmingActionWarm {
		comparison = ">="
	}
	return fmt.Sprintf("%s, expected savings %s %s $%s", probabilityText, formatDollars(decision.ExpectedSavings), comparison, jsstring.ToFixed(cacheWarmingMinimumExpectedSavings, 3))
}

func formatCacheWarmingDecisionTime(nextWarmAt, now int64) string {
	if nextWarmAt == 0 || nextWarmAt <= now {
		return "Decision now"
	}
	remainingSeconds := int64(math.Ceil(float64(nextWarmAt-now) / 1000))
	hours := remainingSeconds / 3600
	minutes := remainingSeconds % 3600 / 60
	seconds := remainingSeconds % 60
	var parts []string
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return "Decision in " + strings.Join(parts, " ")
}

// FormatCacheWarmingStatus is the one-line status /session shows. now is a
// Unix millisecond time.
func FormatCacheWarmingStatus(status CacheWarmingStatus, now int64) string {
	decision := status.Decision
	// A decision is attached once Pi (or an extension) acted on it; "inactive"
	// without one never got that far.
	if decision == nil || (status.State == "inactive" && !decision.EconomicsAvailable && !status.ExtensionOverride) {
		reason := status.Reason
		if reason == "" {
			reason = "unknown reason"
		}
		return "Inactive (" + reason + ")"
	}
	details := formatCacheWarmingEconomics(*decision) + " -> " + string(decision.Action)
	if status.ExtensionOverride {
		details = "extension override, " + formatCacheWarmingEconomics(*decision)
	}
	switch status.State {
	case "inactive":
		return "Stopped (" + details + ")"
	case "refreshing":
		return "Warming cache (" + details + ")"
	}
	return formatCacheWarmingDecisionTime(status.NextWarmAt, now) + " (" + details + ")"
}

// FormatCacheWarmingUsage is the one-line transcript text for persisted
// cache-warming usage.
func FormatCacheWarmingUsage(entry UsageEntry) string {
	note := ""
	if entry.Note != "" {
		note = " (" + entry.Note + ")"
	}
	cost := jsstring.ToFixed(entry.Usage.Cost.Total, 6)
	// Keep at least three decimals and drop trailing zeros after them.
	if dot := strings.IndexByte(cost, '.'); dot >= 0 && len(cost) > dot+4 {
		cost = cost[:dot+4] + strings.TrimRight(cost[dot+4:], "0")
	}
	return "Cache warmed" + note + ": $" + cost
}
