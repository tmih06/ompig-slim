package coding

// Ports packages/coding-agent/src/core/agent-session.ts.

import (
	"context"
	"fmt"
	"slices"

	"github.com/MichaelKinsy/PiG/ai"
)

// CycleModel selects the next available model. Empty direction means forward; only Persist changes global defaults.
func (s *Session) CycleModel(direction string, options ...ModelMutationOptions) (*ModelCycleResult, error) {
	result, complete, err := s.BeginModelCycle(context.Background(), direction, options...)
	if complete != nil {
		complete()
	}
	if result != nil {
		// agent-session.ts:2567 reads thinkingLevel after awaiting model_select, so a handler's change shows.
		result.ThinkingLevel = s.ThinkingLevel()
	}
	return result, err
}

// BeginModelCycle is [Session.CycleModel] split at its await: it applies the model and thinking change, returns the result, and returns the model_select notification as a completion the caller invokes once, or nil when none is pending. RPC admits its next command after this prefix. A nil result means there is nothing to cycle to.
func (s *Session) BeginModelCycle(ctx context.Context, direction string, options ...ModelMutationOptions) (*ModelCycleResult, func(), error) {
	return s.beginModelCycle(ctx, direction, false, options...)
}

// BeginModelCycleOnMain is [Session.BeginModelCycle] for an owner loop, as [Session.SetModelOnMain] is for SetModel: the model and
// thinking change apply on the caller, and the completion runs the thinking_level_select and model_select notifications, which admit
// extension handlers and must not run on the loop. The completion then records in the result the thinking level a handler left
// (agent-session.ts:2567). The caller invokes a non-nil completion once, off the loop. A nil result means there is nothing to cycle to.
// The notifications end when ctx or the Session ends, as [Session.SetModelOnMain]'s do.
func (s *Session) BeginModelCycleOnMain(ctx context.Context, direction string, options ModelMutationOptions) (*ModelCycleResult, func(), error) {
	ctx, release := s.notificationContext(ctx)
	result, complete, err := s.beginModelCycle(ctx, direction, true, options)
	if result == nil || err != nil {
		release()
		return result, complete, err
	}
	return result, func() {
		defer release()
		if complete != nil {
			complete()
		}
		result.ThinkingLevel = s.ThinkingLevel()
	}, nil
}

func (s *Session) beginModelCycle(ctx context.Context, direction string, deferThinking bool, options ...ModelMutationOptions) (*ModelCycleResult, func(), error) {
	if direction != "" && direction != "forward" && direction != "backward" {
		return nil, nil, fmt.Errorf("unknown model cycle direction %q", direction)
	}
	return s.beginRoleCycle(ctx, direction, deferThinking, s.SettingsManager().ModelRoleCycle(), options...)
}

// CycleThinkingLevel advances through supported levels, returning an empty level for a non-reasoning model.
func (s *Session) CycleThinkingLevel(options ...ModelMutationOptions) (ai.ModelThinkingLevel, error) {
	if !s.SupportsThinking() {
		return "", nil
	}
	levels := s.AvailableThinkingLevels()
	if len(levels) > 1 {
		levels = append([]ai.ModelThinkingLevel{ai.ThinkingOff, "auto"}, levels[1:]...)
	}
	next := levels[(slices.Index(levels, s.ConfiguredThinkingLevel())+1)%len(levels)]
	if err := s.SetThinkingLevel(next, options...); err != nil {
		return "", err
	}
	return next, nil
}
