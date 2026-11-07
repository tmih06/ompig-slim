package ai_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestUsageRouteLearnsProviderAccessForUnlistedModel(t *testing.T) {
	store := newFakeAccountStore(accountInfo("openai-codex", keyOf("plus"), true), accountInfo("openai-codex", keyOf("free"), false))
	for _, plan := range []string{"plus", "free"} {
		store.put("openai-codex", keyOf(plan), ai.Credential{Type: ai.CredentialOAuth, Access: codexToken(plan)})
	}
	router := ai.NewAccountRouter(store)
	now := time.Now()
	router.SetClock(func() time.Time { return now })
	model := okModel("openai-codex", "future-model-with-no-catalog-rule")
	request := ai.AccountRouteRequest{SessionID: "live-usage-session"}
	if _, present := router.PeekRoutedAccountKey("openai-codex", request.SessionID, model.ID); present {
		t.Fatal("unseen model had a fabricated account")
	}
	denyFree := true
	var tried []string
	var visible []bool
	attempt := func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
		key, present := router.PeekRoutedAccountKey("openai-codex", request.SessionID, model.ID)
		if present && (key == nil || *key != *selection.AccountKey) {
			return nil, fmt.Errorf("usage identity did not follow live attempt: %v %v", key, present)
		}
		visible = append(visible, present)
		tried = append(tried, *selection.AccountKey)
		if denyFree && *selection.AccountKey == "free" {
			observe(ai.ProviderResponse{Status: 400})
			return emptyErrorStream(fmt.Sprintf("The '%s' model is not supported when using Codex with a ChatGPT account.", model.ID)), nil
		}
		return okStream(), nil
	}
	text, _ := collectStream(t, router.StreamRoutes(t.Context(), model, request, attempt))
	if text != "hello" || len(tried) != 2 || tried[0] != "free" || tried[1] != "plus" {
		t.Fatalf("provider denial did not drive fallback: %q %v", text, tried)
	}
	if visible[0] || visible[1] {
		t.Fatal("unconfirmed provider attempts became visible")
	}
	if key, present := router.PeekRoutedAccountKey("openai-codex", request.SessionID, model.ID); !present || key == nil || *key != "plus" {
		t.Fatal("usage did not retain the successful paid account")
	}
	if _, present := router.PeekRoutedAccountKey("openai-codex", request.SessionID, "another-unlisted-model"); present {
		t.Fatal("routing identity leaked across models")
	}
	// A provider denial is scoped to this account/model and respected on the next request.
	tried, visible = nil, nil
	_, _ = collectStream(t, router.StreamRoutes(t.Context(), model, request, attempt))
	if len(tried) != 1 || tried[0] != "plus" || !visible[0] {
		t.Fatalf("rejected free account was retried before its block expired: %v", tried)
	}
	// Access may change: after the provider block expires, a successful free attempt updates usage without any catalog edit.
	now = now.Add(2 * time.Minute)
	denyFree = false
	tried, visible = nil, nil
	text, _ = collectStream(t, router.StreamRoutes(t.Context(), model, request, attempt))
	if text != "hello" || len(tried) != 1 || tried[0] != "free" || visible[0] {
		t.Fatalf("provider access change required a model-name rule: %q %v", text, tried)
	}
	if key, present := router.PeekRoutedAccountKey("openai-codex", request.SessionID, model.ID); !present || key == nil || *key != "free" {
		t.Fatal("accepted free route was not confirmed")
	}
}

func TestUsageRouteOverlappingAttemptCleanup(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprintf("replace_store_%v", replace), func(t *testing.T) {
			store := newFakeAccountStore(accountInfo("p", keyOf("a"), true), accountInfo("p", keyOf("b"), false))
			router := ai.NewAccountRouter(store)
			model := okModel("p", "future-model")
			request := ai.AccountRouteRequest{SessionID: "s"}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			start := func() (chan struct{}, chan struct{}, chan error) {
				entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
				go func() {
					_, err := router.ImagesRoutes(ctx, model, request, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
						close(entered)
						select {
						case <-release:
							return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
						case <-attemptCtx.Done():
							return ai.AssistantImages{}, context.Cause(attemptCtx)
						}
					})
					done <- err
				}()
				return entered, release, done
			}
			wait := func(entered <-chan struct{}) {
				t.Helper()
				select {
				case <-entered:
				case <-time.After(2 * time.Second):
					t.Fatal("attempt did not start")
				}
			}
			first, releaseFirst, doneFirst := start()
			wait(first)
			if replace {
				router.SetStore(newFakeAccountStore(accountInfo("p", keyOf("b"), true)))
			} else {
				store.block("p", keyOf("a"), "", time.Now().Add(time.Hour).UnixMilli())
			}
			second, releaseSecond, doneSecond := start()
			wait(second)
			close(releaseFirst)
			if err := <-doneFirst; err != nil {
				t.Fatal(err)
			}
			if _, present := router.PeekRoutedAccountKey("p", "s", model.ID); present {
				t.Fatal("old cleanup exposed an account while a newer unverified attempt was pending")
			}
			close(releaseSecond)
			if err := <-doneSecond; err != nil {
				t.Fatal(err)
			}
			if key, present := router.PeekRoutedAccountKey("p", "s", model.ID); !present || key == nil || *key != "b" {
				t.Fatal("successful route was lost after attempt cleanup")
			}
		})
	}
}

func TestUsageRouteCancellationDoesNotLeaveAnAccount(t *testing.T) {
	store := newFakeAccountStore(accountInfo("p", keyOf("a"), true), accountInfo("p", keyOf("b"), false))
	router := ai.NewAccountRouter(store)
	model := okModel("p", "future-model")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started, done := make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := router.ImagesRoutes(ctx, model, ai.AccountRouteRequest{SessionID: "s"}, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
			close(started)
			<-attemptCtx.Done()
			return ai.AssistantImages{}, context.Cause(attemptCtx)
		})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("attempt did not start")
	}
	if _, present := router.PeekRoutedAccountKey("p", "s", model.ID); present {
		t.Fatal("unverified attempt became visible")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled attempt did not drain")
	}
	if _, present := router.PeekRoutedAccountKey("p", "s", model.ID); present {
		t.Fatal("cancelled route remained visible")
	}
	store.block("p", keyOf("a"), "", time.Now().Add(time.Hour).UnixMilli())
	_, err := router.ImagesRoutes(t.Context(), model, ai.AccountRouteRequest{SessionID: "s"}, func(context.Context, ai.AccountSelection, ai.AccountObserve, bool) (ai.AssistantImages, error) {
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if key, present := router.PeekRoutedAccountKey("p", "s", model.ID); !present || key == nil || *key != "b" {
		t.Fatal("a cancelled attempt masked a later confirmed account")
	}
}

func TestUsageRouteHidesPreviouslySuccessfulRejectedAccount(t *testing.T) {
	router := ai.NewAccountRouter(newFakeAccountStore(accountInfo("p", keyOf("a"), true)))
	now := time.Now()
	router.SetClock(func() time.Time { return now })
	model := okModel("p", "future-model")
	request := ai.AccountRouteRequest{SessionID: "s"}
	_, err := router.ImagesRoutes(t.Context(), model, request, func(context.Context, ai.AccountSelection, ai.AccountObserve, bool) (ai.AssistantImages, error) {
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = router.ImagesRoutes(t.Context(), model, request, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
		observe(ai.ProviderResponse{Status: 429})
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonError, ErrorMessage: "insufficient_quota"}, nil
	})
	if _, present := router.PeekRoutedAccountKey("p", "s", model.ID); present {
		t.Fatal("a known rejected account remained on the bar")
	}
	now = now.Add(2 * time.Minute)
	if _, present := router.PeekRoutedAccountKey("p", "s", model.ID); present {
		t.Fatal("retry expiry revived a rejected account without a new route")
	}
}

func TestUsageRouteConfirmsAccountWhileStreaming(t *testing.T) {
	router := ai.NewAccountRouter(newFakeAccountStore(accountInfo("p", keyOf("a"), true)))
	model := okModel("p", "unlisted-streaming-model")
	source := ai.NewAssistantMessageEventStream()
	partial := &ai.AssistantMessage{Provider: "p", Model: model.ID, Content: []ai.AssistantContentBlock{ai.TextContent{Text: "hello"}}}
	_ = source.Push(ai.StartEvent{Partial: partial})
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	opened := make(chan struct{})
	stream := router.StreamRoutes(ctx, model, ai.AccountRouteRequest{SessionID: "s"}, func(context.Context, ai.AccountSelection, ai.AccountObserve, bool) (*ai.AssistantMessageEventStream, error) {
		close(opened)
		return source, nil
	})
	select {
	case <-opened:
	case <-ctx.Done():
		t.Fatal("stream did not open")
	}
	if _, present := router.PeekRoutedAccountKey("p", "s", model.ID); present {
		t.Fatal("start-only response was treated as accepted content")
	}
	_ = source.Push(ai.TextDeltaEvent{ContentIndex: 0, Delta: "hello", Partial: partial})
	received := false
	for event := range stream.Events(ctx) {
		if _, ok := event.(ai.TextDeltaEvent); ok {
			received = true
			if key, present := router.PeekRoutedAccountKey("p", "s", model.ID); !present || key == nil || *key != "a" {
				t.Fatal("accepted live content did not update usage before completion")
			}
			_ = source.Push(ai.DoneEvent{Reason: ai.StopReasonStop, Message: &ai.AssistantMessage{Provider: "p", Model: model.ID, Content: partial.Content, StopReason: ai.StopReasonStop}})
		}
	}
	if !received {
		t.Fatal("stream did not deliver content")
	}
}

func TestUsageRouteDenialDoesNotInvalidateAnotherModel(t *testing.T) {
	store := newFakeAccountStore(accountInfo("openai-codex", keyOf("a"), true))
	store.put("openai-codex", keyOf("a"), ai.Credential{Type: ai.CredentialOAuth, Access: codexToken("free")})
	router := ai.NewAccountRouter(store)
	first, second := okModel("openai-codex", "future-one"), okModel("openai-codex", "future-two")
	request := ai.AccountRouteRequest{SessionID: "s"}
	for _, model := range []*ai.Model{first, second} {
		_, err := router.ImagesRoutes(t.Context(), model, request, func(context.Context, ai.AccountSelection, ai.AccountObserve, bool) (ai.AssistantImages, error) {
			return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, _ = router.ImagesRoutes(t.Context(), first, request, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
		observe(ai.ProviderResponse{Status: 400})
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonError, ErrorMessage: "The 'future-one' model is not supported when using Codex with a ChatGPT account."}, nil
	})
	if _, present := router.PeekRoutedAccountKey("openai-codex", "s", first.ID); present {
		t.Fatal("rejected model retained its bar")
	}
	if _, present := router.PeekRoutedAccountKey("openai-codex", "s", second.ID); !present {
		t.Fatal("model-specific denial invalidated an unrelated model")
	}
}

func TestUsageRouteOldStoreCompletionCannotRestoreBar(t *testing.T) {
	router := ai.NewAccountRouter(newFakeAccountStore(accountInfo("p", keyOf("old"), true)))
	model := okModel("p", "future-model")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := router.ImagesRoutes(ctx, model, ai.AccountRouteRequest{SessionID: "s"}, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
			close(entered)
			select {
			case <-release:
				return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
			case <-attemptCtx.Done():
				return ai.AssistantImages{}, context.Cause(attemptCtx)
			}
		})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("attempt did not start")
	}
	router.SetStore(newFakeAccountStore(accountInfo("p", keyOf("new"), true)))
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, present := router.PeekRoutedAccountKey("p", "s", model.ID); present {
		t.Fatal("outgoing credentials restored a route in the new store")
	}
}
