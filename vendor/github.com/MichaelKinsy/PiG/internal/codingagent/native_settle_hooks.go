package codingagent

// Native settle-boundary hooks: before-settle work that belongs to the harness
// rather than to an extension.
//
// A built-in tool registers a hook here from its init, the way it registers its
// definition with internal/codingagent/tools. The Session consults the hooks at
// the same point it dispatches agent_before_settle to extensions, so a native
// hook needs no extension, never appears in the startup Extensions list, and is
// unaffected by --no-extensions.
//
// Why this exists: the todo reminder must run before settle, and before settle is
// otherwise reachable only through extension.API. Upstream has no equivalent
// registry because its built-ins live in core/tools and call the session
// directly; PiG's built-ins live in packages that cannot import `coding` (which
// imports this package), so they register here instead.

import (
	"context"
	"sync"

	"github.com/MichaelKinsy/PiG/internal/sessionentry"
)

// NativeSettleState is what a hook reads and the one way it may act.
type NativeSettleState struct {
	// Outcome is the finished run's activity outcome, as an AgentActivityOutcome
	// value ("completed", "aborted", "error"). A hook normally continues only on
	// "completed": an aborted or failed run was not the model deciding it finished.
	Outcome string

	// Branch is the session's current branch, oldest entry first.
	Branch []sessionentry.SessionEntry

	// LastAssistantText is the text of the run's final assistant message, or ""
	// when the run produced none.
	LastAssistantText string

	// Deliver appends a hidden custom message the model sees and the user does not,
	// and continues the run so the model acts on it. It is nil only in a session
	// that cannot accept messages; a hook that returns true without delivering
	// continues the run with no explanation, so call it first.
	Deliver func(customType, content string)
}

// NativeSettleHook decides whether a run should continue instead of settling.
//
// It runs after the extension agent_before_settle handlers and before the final
// context check. Returning true continues the run, exactly as an extension
// handler returning Continue does, so a hook must have delivered a message that
// explains why. A hook runs on every potential settle and must be cheap.
type NativeSettleHook func(ctx context.Context, state NativeSettleState) bool

var (
	nativeSettleMu    sync.RWMutex
	nativeSettleHooks []NativeSettleHook
)

// RegisterNativeSettleHook adds a before-settle hook, in registration order.
// It is meant for a package's init; it is safe to call concurrently, and a nil
// hook is ignored.
func RegisterNativeSettleHook(hook NativeSettleHook) {
	if hook == nil {
		return
	}
	nativeSettleMu.Lock()
	defer nativeSettleMu.Unlock()
	nativeSettleHooks = append(nativeSettleHooks, hook)
}

// NativeSettleHooks returns the registered hooks, in registration order.
func NativeSettleHooks() []NativeSettleHook {
	nativeSettleMu.RLock()
	defer nativeSettleMu.RUnlock()
	out := make([]NativeSettleHook, len(nativeSettleHooks))
	copy(out, nativeSettleHooks)
	return out
}

// HasNativeSettleHooks reports whether any hook is registered, so the Session can
// skip the boundary pass exactly as it skips it when no extension subscribed.
func HasNativeSettleHooks() bool {
	nativeSettleMu.RLock()
	defer nativeSettleMu.RUnlock()
	return len(nativeSettleHooks) > 0
}
