package extension

import "context"

// OnTurnEndSummary registers an awaited turn handler that needs the completed
// message, tool results, outcome and prior drafts, but not the projected history.
// Context is always empty. Ordering, draft validation, continuation and errors
// are identical to OnTurnEnd. Hosts without the summary path use ordinary
// dispatch with the context removed; this keeps external API implementations
// compatible without making partial previews observable to normal handlers.
func OnTurnEndSummary(api API, handler func(context.Context, TurnEndEvent) (TurnEndEventResult, error)) func() {
	if summaries, ok := api.(interface {
		OnTurnEndSummary(func(context.Context, TurnEndEvent) (TurnEndEventResult, error)) func()
	}); ok {
		return summaries.OnTurnEndSummary(handler)
	}
	return api.OnTurnEnd(func(ctx context.Context, event TurnEndEvent) (TurnEndEventResult, error) {
		original := event.BoundaryState
		state := *original
		state.Context = BoundaryContextPreview{}
		event.BoundaryState = &state
		result, err := handler(ctx, event)
		original.Entries, original.Continue = state.Entries, state.Continue
		return result, err
	})
}
