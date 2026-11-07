package ai

// The routed chat, image and classifier operations of an AccountRouter. Each drives the same
// candidate list, block check, same-account OAuth refresh retry and rotation policy, and each
// keeps one hard boundary: nothing a failed attempt already delivered is ever replayed.

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// StreamRoutes runs one chat request across the provider's stored accounts and returns at once
// with the stream of the attempt that produced a response.
//
// Only a rejection that arrives before any content escapes is retried on a sibling account: the
// attempt's start event is held until a real event follows it, and an error event with no
// content, no abort and no caller cancellation blocks the account and rotates. Once any content
// is emitted, a failure is reported as it is — rotating would replay a partial answer.
//
// A request the router does not route opens once against the provider's selected account,
// exactly as an unpooled request.
func (r *AccountRouter) StreamRoutes(ctx context.Context, model AnyModel, request AccountRouteRequest, open AccountAttemptStream) *AssistantMessageEventStream {
	outer := NewAssistantMessageEventStream()
	ctx = outer.ObservationContext(ctx)
	go r.streamRoutes(ctx, outer, model, request, open)
	return outer
}

func (r *AccountRouter) streamRoutes(ctx context.Context, outer *AssistantMessageEventStream, model AnyModel, request AccountRouteRequest, open AccountAttemptStream) {
	accounts, err := r.Candidates(ctx, model, request)
	if err != nil {
		if context.Cause(ctx) != nil {
			err = AbortReason(ctx)
		}
		pushAccountSetupError(outer, model, err)
		return
	}
	if accounts == nil {
		// An unpooled request still consumes its provider through the bounded attempt reader, so a
		// provider that closes without resolving a result cannot hang the consumer here.
		observed := newObservedResponse()
		stream, openErr := open(ctx, AccountSelection{}, observed.record, false)
		if openErr != nil {
			if context.Cause(ctx) != nil {
				openErr = AbortReason(ctx)
			}
			pushAccountSetupError(outer, model, openErr)
			return
		}
		if stream == nil {
			pushAccountSetupError(outer, model, errors.New("provider returned no stream"))
			return
		}
		r.driveAttempt(ctx, outer, model, nil, request, stream, observed, false)
		return
	}
	var lastFailure *ErrorEvent
	var lastSetupError error
	var earliest *int64
	var previous *accountFailure
	for _, account := range accounts {
		if context.Cause(ctx) != nil {
			pushAccountSetupError(outer, model, AbortReason(ctx))
			return
		}
		until, blockErr := r.BlockedUntil(ctx, model, account)
		if blockErr != nil {
			pushAccountSetupError(outer, model, blockErr)
			return
		}
		if until != nil && *until > r.nowMillis() {
			earliest = earliestDeadline(earliest, *until)
			continue
		}
		if previous != nil {
			r.reportRotation(model.ProviderID(), model.ModelID(), *previous, account, request)
		}
		for refresh := range 2 {
			attemptCtx, selection := r.attemptContext(ctx, account)
			observed := newObservedResponse()
			attemptCtx = withAccountResponseObserver(attemptCtx, observed.record)
			stream, openErr := open(attemptCtx, selection, observed.record, refresh == 1)
			if openErr != nil {
				if context.Cause(ctx) != nil {
					pushAccountSetupError(outer, model, AbortReason(ctx))
					return
				}
				block := r.Fail(ctx, model, account, accountErrorText(openErr), observed.get())
				if block == nil {
					pushAccountSetupError(outer, model, openErr)
					return
				}
				previous = &accountFailure{account: account, scope: block.Scope}
				lastFailure, lastSetupError = nil, openErr
				break
			}
			if stream == nil {
				pushAccountSetupError(outer, model, errors.New("provider returned no stream"))
				return
			}
			outcome := r.driveAttempt(ctx, outer, model, account, request, stream, observed, refresh == 1)
			lastFailure, lastSetupError = outcome.event, nil
			switch {
			case outcome.delivered:
				return
			case outcome.retryRefresh:
				continue
			case outcome.rotated:
				previous = outcome.failure
			}
			break
		}
	}
	if lastFailure != nil {
		_ = outer.Push(*lastFailure)
		return
	}
	if lastSetupError != nil {
		pushAccountSetupError(outer, model, lastSetupError)
		return
	}
	pushAccountSetupError(outer, model, errors.New(accountUnavailableMessage(model.ProviderID(), model.ModelID(), earliest)))
}

// accountStreamOutcome is what one attempt's event run decided.
type accountStreamOutcome struct {
	// emitted reports that content reached the consumer, so nothing may replay.
	emitted bool
	// delivered reports that the attempt was delivered and the request is finished.
	delivered bool
	// retryRefresh reports that the same account must be retried after a forced token refresh.
	retryRefresh bool
	// rotated reports that this account was retired and a sibling should be tried.
	rotated bool
	failure *accountFailure
	event   *ErrorEvent
}

// driveAttempt consumes one attempt's stream, holding its start event until a real event follows.
func (r *AccountRouter) driveAttempt(ctx context.Context, outer *AssistantMessageEventStream, model AnyModel, account *CredentialAccountInfo, request AccountRouteRequest, stream *AssistantMessageEventStream, observed *observedResponse, forceOAuthRefresh bool) accountStreamOutcome {
	var start *StartEvent
	// partial is the last message the consumer saw, so a cancellation terminal reports the same
	// content instead of discarding it.
	var partial *AssistantMessage
	emitted := false
	terminal := false
	for event := range stream.Events(ctx) {
		if emittedContent, ok := event.(StartEvent); ok && !emitted {
			copied := emittedContent
			start = &copied
			partial = copied.Partial
			continue
		}
		if current := eventPartial(event); current != nil {
			partial = current
		}
		if failure, ok := event.(ErrorEvent); ok && !emitted && errorEventIsEmpty(failure) && failure.Reason != StopReasonAborted && context.Cause(ctx) == nil && account != nil {
			if shouldRetryWithRefresh(account, forceOAuthRefresh, observed.get()) {
				return accountStreamOutcome{retryRefresh: true}
			}
			block := r.Fail(ctx, model, account, errorMessageOf(failure), observed.get())
			if block != nil {
				return accountStreamOutcome{rotated: true, failure: &accountFailure{account: account, scope: block.Scope}, event: &failure}
			}
		}
		if start != nil {
			_ = outer.Push(*start)
			start = nil
			emitted = true
		}
		emitted = true
		switch event.(type) {
		case DoneEvent:
			terminal = true
			// A successful turn pins the account for this session and model (account-pool.ts
			// stream `if (event.type === "done") this.pin(...)`).
			if account != nil {
				r.pin(model.ProviderID(), model.ModelID(), account.AccountKey, request)
			}
		case ErrorEvent:
			terminal = true
		}
		_ = outer.Push(event)
	}
	if terminal {
		// A terminal event resolved the stream's result before it reached the consumer. Ending
		// without a result would leave the consumer's Result pending forever, so an unreachable
		// resolution still ends with a terminal message.
		if result, ok := stream.ResultResolved(); ok && result != nil {
			outer.End(result)
		} else {
			_ = outer.Push(ErrorEvent{Reason: StopReasonError, Error: accountTerminalMessage(model, errors.New("provider stream ended without a result"), StopReasonError)})
		}
		return accountStreamOutcome{emitted: true, delivered: true}
	}
	// The attempt's event iterator closed without a terminal event. A provider that ended its
	// iterator after ending the stream still has a result; one that closed on our cancellation, or
	// ended without any result, does not, and waiting for it would hang this router goroutine and
	// the consumer forever (Pi's EventStream.end() with no argument leaves result() pending).
	message, resolved := stream.ResultResolved()
	if !resolved || message == nil {
		// A cancelled request reports its abort; a live request whose provider closed without a
		// result is a stream failure, not a success and not a cancellation.
		if err := AbortReason(ctx); err != nil {
			pushAccountAbort(outer, model, err, StopReasonAborted, partial)
		} else {
			pushAccountAbort(outer, model, errors.New("provider stream ended without a result"), StopReasonError, partial)
		}
		return accountStreamOutcome{emitted: true, delivered: true}
	}
	if message.StopReason == StopReasonError || message.StopReason == StopReasonAborted {
		if emitted || message.StopReason == StopReasonAborted || context.Cause(ctx) != nil {
			_ = outer.Push(ErrorEvent{Reason: message.StopReason, Error: message})
			outer.End(message)
			return accountStreamOutcome{emitted: true, delivered: true}
		}
		event := ErrorEvent{Reason: message.StopReason, Error: message}
		var block *AccountFailureScope
		if account != nil {
			block = r.Fail(ctx, model, account, message.ErrorMessage, observed.get())
		}
		if block != nil {
			return accountStreamOutcome{rotated: true, failure: &accountFailure{account: account, scope: block.Scope}, event: &event}
		}
		_ = outer.Push(event)
		outer.End(message)
		return accountStreamOutcome{emitted: true, delivered: true}
	}
	reason := message.StopReason
	if reason == StopReasonPending {
		reason = StopReasonDeferred
	}
	if start != nil {
		_ = outer.Push(*start)
	}
	_ = outer.Push(DoneEvent{Reason: reason, Message: message})
	outer.End(message)
	if account != nil {
		r.pin(model.ProviderID(), model.ModelID(), account.AccountKey, request)
	}
	return accountStreamOutcome{emitted: true, delivered: true}
}

// pushAccountAbort ends a routed stream that has no usable terminal result: aborted when the
// request was cancelled, an error when the provider closed without a result. It never reports a
// success and never discards content the consumer already saw.
func pushAccountAbort(stream *AssistantMessageEventStream, model AnyModel, reason error, stopReason StopReason, partial *AssistantMessage) {
	var message *AssistantMessage
	if partial != nil {
		// Keep what the consumer already saw: the terminal reports the same content with the real
		// stop reason instead of a fabricated success.
		copied := new(*partial)
		copied.StopReason = stopReason
		copied.ErrorMessage = reason.Error()
		copied.ErrorBeforeTimestamp = true
		message = copied
	} else {
		message = accountTerminalMessage(model, reason, stopReason)
	}
	_ = stream.Push(ErrorEvent{Reason: stopReason, Error: message})
	stream.End(message)
}

// errorEventIsEmpty reports that a failed attempt carried no content, which is what makes a
// sibling account worth trying.
func errorEventIsEmpty(event ErrorEvent) bool {
	return event.Error == nil || len(event.Error.Content) == 0
}

// errorMessageOf is the failure text of an error event.
func errorMessageOf(event ErrorEvent) string {
	if event.Error == nil {
		return ""
	}
	return event.Error.ErrorMessage
}

// ImagesRoutes runs one image-generation request across the provider's stored accounts. A result
// that produced any image is returned as it is; only an error result with no output rotates.
func (r *AccountRouter) ImagesRoutes(ctx context.Context, model AnyModel, request AccountRouteRequest, attempt AccountAttemptImages) (AssistantImages, error) {
	return routeAccountResult(ctx, r, model, request, attempt, func(result AssistantImages) accountOneShotOutcome {
		return accountOneShotOutcome{stopReason: string(result.StopReason), output: len(result.Output) > 0, message: result.ErrorMessage}
	})
}

// ClassifyRoutes runs one classification request across the provider's stored accounts. A result
// that answered any question is returned as it is; only an error result with no answer rotates.
func (r *AccountRouter) ClassifyRoutes(ctx context.Context, model AnyModel, request AccountRouteRequest, attempt AccountAttemptClassify) (ClassifierResult, error) {
	return routeAccountResult(ctx, r, model, request, attempt, func(result ClassifierResult) accountOneShotOutcome {
		return accountOneShotOutcome{stopReason: string(result.StopReason), output: len(result.Answers) > 0, message: result.ErrorMessage}
	})
}

// accountOneShotOutcome is the routing view of one image or classifier result.
type accountOneShotOutcome struct {
	stopReason string
	// output reports that the result carries something the caller would see.
	output  bool
	message string
}

func (outcome accountOneShotOutcome) failed() bool {
	return outcome.stopReason == string(StopReasonError)
}

func (outcome accountOneShotOutcome) success() bool {
	return outcome.stopReason == string(StopReasonStop)
}

// routeAccountResult drives one one-shot operation across the candidate accounts. A failed
// attempt that produced no output rotates; one that produced any output is delivered unchanged.
func routeAccountResult[T any](
	ctx context.Context,
	router *AccountRouter,
	model AnyModel,
	request AccountRouteRequest,
	attempt func(context.Context, AccountSelection, AccountObserve, bool) (T, error),
	adapt func(T) accountOneShotOutcome,
) (T, error) {
	var zero T
	accounts, err := router.Candidates(ctx, model, request)
	if err != nil {
		return zero, err
	}
	if accounts == nil {
		return attempt(ctx, AccountSelection{}, nil, false)
	}
	var last T
	lastFailed := false
	var lastError error
	var previous *accountFailure
	var earliest *int64
	for _, account := range accounts {
		if err := context.Cause(ctx); err != nil {
			return zero, err
		}
		until, blockErr := router.BlockedUntil(ctx, model, account)
		if blockErr != nil {
			return zero, blockErr
		}
		if until != nil && *until > router.nowMillis() {
			earliest = earliestDeadline(earliest, *until)
			continue
		}
		if previous != nil {
			router.reportRotation(model.ProviderID(), model.ModelID(), *previous, account, request)
		}
		for refresh := range 2 {
			attemptCtx, selection := router.attemptContext(ctx, account)
			observed := newObservedResponse()
			attemptCtx = withAccountResponseObserver(attemptCtx, observed.record)
			value, attemptErr := attempt(attemptCtx, selection, observed.record, refresh == 1)
			if attemptErr != nil {
				if err := context.Cause(ctx); err != nil {
					return zero, err
				}
				block := router.Fail(ctx, model, account, accountErrorText(attemptErr), observed.get())
				if block == nil {
					return zero, attemptErr
				}
				previous = &accountFailure{account: account, scope: block.Scope}
				last, lastFailed, lastError = zero, false, attemptErr
				break
			}
			outcome := adapt(value)
			if !outcome.failed() {
				if outcome.success() {
					router.pin(model.ProviderID(), model.ModelID(), account.AccountKey, request)
				}
				return value, nil
			}
			if outcome.output || context.Cause(ctx) != nil {
				return value, nil
			}
			if shouldRetryWithRefresh(account, refresh == 1, observed.get()) {
				continue
			}
			block := router.Fail(ctx, model, account, outcome.message, observed.get())
			if block == nil {
				return value, nil
			}
			previous = &accountFailure{account: account, scope: block.Scope}
			last, lastFailed, lastError = value, true, nil
			break
		}
	}
	if lastFailed {
		return last, nil
	}
	if lastError != nil {
		return zero, lastError
	}
	return zero, errors.New(accountUnavailableMessage(model.ProviderID(), model.ModelID(), earliest))
}

// attemptContext marks the context of one attempt with the account it resolves, so every auth
// resolution the attempt performs selects that account.
func (r *AccountRouter) attemptContext(ctx context.Context, account *CredentialAccountInfo) (context.Context, AccountSelection) {
	selection := AccountSelection{AccountKey: account.AccountKey, AccountKeySet: true}
	return WithAccountRequest(ctx, AccountRequest{AccountKey: account.AccountKey, AccountKeySet: true}), selection
}

// observedResponse records the provider response of the current attempt, so a rejection can be
// classified by status and Retry-After.
type observedResponse struct {
	current atomic.Pointer[ProviderResponse]
}

func newObservedResponse() *observedResponse { return &observedResponse{} }

func (observed *observedResponse) record(response ProviderResponse) {
	copied := response
	observed.current.Store(&copied)
}

func (observed *observedResponse) get() *ProviderResponse {
	if observed == nil {
		return nil
	}
	return observed.current.Load()
}

// pushAccountSetupError ends a routed stream with the setup failure, built the way a lazy setup
// rejection is (errorMessage before timestamp).
func pushAccountSetupError(stream *AssistantMessageEventStream, model AnyModel, err error) {
	_ = stream.Push(ErrorEvent{Reason: StopReasonError, Error: accountTerminalMessage(model, err, StopReasonError)})
}

// accountTerminalMessage builds the terminal message a routed stream reports, errorMessage before
// timestamp as a lazy setup rejection does.
func accountTerminalMessage(model AnyModel, err error, reason StopReason) *AssistantMessage {
	message := &AssistantMessage{Content: []AssistantContentBlock{}, StopReason: reason, ErrorBeforeTimestamp: true, Timestamp: time.Now().UnixMilli()}
	if err != nil {
		message.ErrorMessage = err.Error()
	}
	if model != nil {
		message.Provider = model.ProviderID()
		message.Model = model.ModelID()
		if chat, ok := model.(*Model); ok {
			message.API = chat.ProviderMeta.API
		}
	}
	return message
}
