package ai_test

// Behavioural tests of the request-local account router: model-plan eligibility, transient
// non-rotation, the pre/post-content fallback boundary, and per-model session affinity.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// fakeAccountStore is a multi-account credential store used only by these tests.
type fakeAccountStore struct {
	mu       sync.Mutex
	accounts []ai.CredentialAccountInfo
	creds    map[string]ai.Credential
	blocks   map[string]int64
	blockErr error
	now      func() time.Time
}

func newFakeAccountStore(entries ...ai.CredentialAccountInfo) *fakeAccountStore {
	return &fakeAccountStore{accounts: entries, creds: map[string]ai.Credential{}, blocks: map[string]int64{}, now: time.Now}
}

func credentialID(providerID string, accountKey *string) string {
	if accountKey == nil {
		return providerID + "\x00<none>"
	}
	return providerID + "\x00" + *accountKey
}

func (store *fakeAccountStore) put(providerID string, accountKey *string, credential ai.Credential) {
	store.creds[credentialID(providerID, accountKey)] = credential
}

func (store *fakeAccountStore) Read(ctx context.Context, providerID string) (*ai.Credential, error) {
	for _, account := range store.accounts {
		if account.ProviderID == providerID && account.Active {
			return store.ReadAccount(ctx, providerID, account.AccountKey)
		}
	}
	return nil, nil
}

func (store *fakeAccountStore) List(ctx context.Context) ([]ai.CredentialInfo, error) {
	var out []ai.CredentialInfo
	for _, account := range store.accounts {
		out = append(out, ai.CredentialInfo{ProviderID: account.ProviderID, Type: account.Type})
	}
	return out, nil
}

func (store *fakeAccountStore) Modify(ctx context.Context, providerID string, fn func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
	return nil, errors.New("not used")
}

func (store *fakeAccountStore) Delete(ctx context.Context, providerID string) error { return nil }

func (store *fakeAccountStore) ReadAccount(ctx context.Context, providerID string, accountKey *string) (*ai.Credential, error) {
	credential, ok := store.creds[credentialID(providerID, accountKey)]
	if !ok {
		return nil, nil
	}
	return &credential, nil
}

func (store *fakeAccountStore) ListAccounts(ctx context.Context) ([]ai.CredentialAccountInfo, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return append([]ai.CredentialAccountInfo(nil), store.accounts...), nil
}

func (store *fakeAccountStore) SaveAccount(ctx context.Context, providerID string, accountKey *string, credential ai.Credential) ([]ai.CredentialAccountInfo, error) {
	return nil, errors.New("not used")
}

func (store *fakeAccountStore) SelectAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
	return false, nil
}

func (store *fakeAccountStore) DeleteAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
	return false, nil
}

func (store *fakeAccountStore) ModifyAccount(ctx context.Context, providerID string, accountKey *string, fn func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
	return nil, errors.New("not used")
}

func (store *fakeAccountStore) GetAccountBlockedUntil(ctx context.Context, providerID string, accountKey *string, scopes []string) (*int64, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	until := int64(0)
	for _, scope := range append([]string{""}, scopes...) {
		if deadline := store.blocks[credentialID(providerID, accountKey)+"\x00"+scope]; deadline > until {
			until = deadline
		}
	}
	if until == 0 {
		return nil, nil
	}
	return &until, nil
}

func (store *fakeAccountStore) BlockAccount(ctx context.Context, providerID string, accountKey *string, scope string, untilMs int64) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	key := credentialID(providerID, accountKey) + "\x00" + scope
	if untilMs > store.blocks[key] {
		store.blocks[key] = untilMs
	}
	return store.blockErr
}

func (store *fakeAccountStore) AccountCount() (int, error) { return len(store.accounts), nil }

// block records a block directly, as another process would have written it.
func (store *fakeAccountStore) block(providerID string, accountKey *string, scope string, untilMs int64) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.blocks[credentialID(providerID, accountKey)+"\x00"+scope] = untilMs
}

func keyOf(value string) *string { return new(value) }

func accountInfo(providerID string, accountKey *string, active bool) ai.CredentialAccountInfo {
	return ai.CredentialAccountInfo{ProviderID: providerID, Type: ai.CredentialOAuth, AccountKey: accountKey, Active: active}
}

// codexToken builds an unsigned JWT whose ChatGPT plan claim is plan.
func codexToken(plan string) string {
	encode := func(text string) string { return base64.RawURLEncoding.EncodeToString([]byte(text)) }
	payload := fmt.Sprintf(`{"https://api.openai.com/auth":{"chatgpt_plan_type":%q}}`, plan)
	return encode(`{"alg":"none"}`) + "." + encode(payload) + ".signature"
}

// okModel builds a chat model identity the router routes on.
func okModel(provider, id string) *ai.Model {
	return &ai.Model{ID: id, ProviderMeta: ai.ProviderMetadata{ProviderID: provider, API: ai.APIOpenAICompletions}}
}

func TestAccountMeetsModelPlan(t *testing.T) {
	codex := func(plan string) ai.Credential {
		return ai.Credential{Type: ai.CredentialOAuth, Access: codexToken(plan)}
	}
	free := codex("free")

	// A free plan cannot serve a paid catalog model, and the gate rejects it before any request.
	if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-sol", free); meets == nil || *meets {
		t.Fatalf("free plan must be ineligible for gpt-5.6-sol, got %v", meets)
	}
	// A pro plan can.
	if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-sol", codex("pro")); meets == nil || !*meets {
		t.Fatalf("pro plan must be eligible for gpt-5.6-sol, got %v", meets)
	}
	// Plus serves paid models but not a Spark model, which needs pro.
	if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-luna", codex("plus")); meets == nil || !*meets {
		t.Fatalf("plus plan must be eligible for gpt-5.6-luna, got %v", meets)
	}
	if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-spark", codex("plus")); meets == nil || *meets {
		t.Fatalf("plus plan must be ineligible for a Spark model, got %v", meets)
	}
	// Any plan serves a model with no catalog requirement, free included.
	if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5", free); meets == nil || !*meets {
		t.Fatalf("a free plan must serve an unrestricted model, got %v", meets)
	}
	// An unreadable plan stays probeable rather than eligible.
	if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-sol", ai.Credential{Type: ai.CredentialOAuth, Access: "not-a-jwt"}); meets != nil {
		t.Fatalf("an unreadable plan must stay unknown, got %v", meets)
	}
	// A different provider and an API key carry no plan.
	if meets := ai.AccountMeetsModelPlan("openai", "gpt-6.1-sol", free); meets == nil || !*meets {
		t.Fatalf("another provider is always eligible, got %v", meets)
	}
	if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-6.1-sol", ai.Credential{Type: ai.CredentialAPIKey, Key: "sk-test"}); meets == nil || !*meets {
		t.Fatalf("an API key is always eligible, got %v", meets)
	}
}

func TestAccountFailureBlockClassifies(t *testing.T) {
	now := time.Now().UnixMilli()
	quota := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "usage_limit_reached", 429, nil)
	if quota == nil || quota.Scope != "chat" || quota.Until < now+59_000 {
		t.Fatalf("quota failure must block chat for at least a minute, got %+v", quota)
	}
	// Transient throttling is not an account failure: a sibling would not help.
	if block := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "too many requests, please slow down", 429, nil); block != nil {
		t.Fatalf("transient 429 must not rotate, got %+v", block)
	}
	if block := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "internal server error", 500, nil); block != nil {
		t.Fatalf("server failure must not rotate, got %+v", block)
	}
	// A Spark model keeps its own meter.
	if spark := ai.AccountFailureBlock("openai-codex", "gpt-5.6-spark", "insufficient_quota", 200, nil); spark == nil || spark.Scope != "spark" {
		t.Fatalf("a Spark quota failure must use the spark scope, got %+v", spark)
	}
	// An auth rejection blocks the auth scope for a minute.
	if auth := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "invalidated oauth token", 401, nil); auth == nil || auth.Scope != "auth" {
		t.Fatalf("an auth rejection must use the auth scope, got %+v", auth)
	}
	// The plan denial is scoped to the model, not the account.
	denied := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", `The 'gpt-6.1-sol' model is not supported when using Codex with a ChatGPT account.`, 400, nil)
	if denied == nil || denied.Scope != "model-policy:gpt-6.1-sol" {
		t.Fatalf("a plan denial must use the model-policy scope, got %+v", denied)
	}
	// The provider's own wait outranks the one-minute floor.
	if slow := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "usage_limit_reached; try again in 5 min", 429, nil); slow == nil || slow.Until < now+299_000 {
		t.Fatalf("a stated retry wait must extend the block, got %+v", slow)
	}
}

func TestRouterTransientFailureDoesNotRotate(t *testing.T) {
	store := newFakeAccountStore(
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", keyOf("b"), false),
	)
	router := ai.NewAccountRouter(store)
	model := okModel("p", "m")
	var tried []string
	result, err := router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
		tried = append(tried, *selection.AccountKey)
		return ai.AssistantImages{}, errors.New("connection reset by peer")
	})
	if err == nil {
		t.Fatal("a transport failure must surface as an error")
	}
	if len(tried) != 1 || tried[0] != "a" {
		t.Fatalf("a transient failure must not rotate accounts, tried %v", tried)
	}
	if result.StopReason != "" {
		t.Fatalf("expected no result, got %+v", result)
	}
}

func TestRouterRotatesOnQuotaBeforeOutput(t *testing.T) {
	store := newFakeAccountStore(
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", keyOf("b"), false),
	)
	router := ai.NewAccountRouter(store)
	model := okModel("p", "m")
	var tried []string
	result, err := router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
		tried = append(tried, *selection.AccountKey)
		if *selection.AccountKey == "a" {
			observe(ai.ProviderResponse{Status: 429, Headers: map[string]string{}})
			return ai.AssistantImages{StopReason: ai.ImagesStopReasonError, ErrorMessage: "insufficient_quota"}, nil
		}
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop, Output: []ai.ImagesOutputContent{ai.TextContent{Text: "ok"}}}, nil
	})
	if err != nil {
		t.Fatalf("rotation must hide the first account's rejection: %v", err)
	}
	if len(tried) != 2 || tried[0] != "a" || tried[1] != "b" {
		t.Fatalf("expected a then b, got %v", tried)
	}
	if result.StopReason != ai.ImagesStopReasonStop {
		t.Fatalf("expected the second account's result, got %+v", result)
	}
	until, err := store.GetAccountBlockedUntil(context.Background(), "p", keyOf("a"), ai.AccountRequestBlockScopes("p", "m"))
	if err != nil || until == nil || *until <= time.Now().UnixMilli() {
		t.Fatalf("the rejected account must stay blocked: %v %v", until, err)
	}
}

func TestRouterKeepsPartialOutput(t *testing.T) {
	store := newFakeAccountStore(
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", keyOf("b"), false),
	)
	router := ai.NewAccountRouter(store)
	model := okModel("p", "m")
	var tried []string
	result, _ := router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
		tried = append(tried, *selection.AccountKey)
		// The account reports quota exhaustion, but one image was already produced: replaying
		// would bill the work twice and discard output the caller can use.
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonError, ErrorMessage: "insufficient_quota", Output: []ai.ImagesOutputContent{ai.TextContent{Text: "partial"}}}, nil
	})
	if len(tried) != 1 {
		t.Fatalf("a partial result must not rotate, tried %v", tried)
	}
	if len(result.Output) != 1 {
		t.Fatalf("the partial result must be delivered, got %+v", result)
	}
}

func TestRouterHonorsPersistedBlocksAcrossProcesses(t *testing.T) {
	store := newFakeAccountStore(
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", keyOf("b"), false),
	)
	// Another process blocked the selected account; a fresh router must honor it immediately.
	store.block("p", keyOf("a"), "auth", time.Now().Add(time.Hour).UnixMilli())
	router := ai.NewAccountRouter(store)
	model := okModel("p", "m")
	var tried []string
	_, _ = router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
		tried = append(tried, *selection.AccountKey)
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
	})
	if len(tried) != 1 || tried[0] != "b" {
		t.Fatalf("a persisted block must keep the account out of the candidate list, tried %v", tried)
	}
}

func TestRouterAffinityPerModel(t *testing.T) {
	store := newFakeAccountStore(
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", keyOf("b"), false),
	)
	router := ai.NewAccountRouter(store)
	request := ai.AccountRouteRequest{SessionID: "s1"}
	first := okModel("p", "m1")
	second := okModel("p", "m2")
	var tried []string
	attempt := func(accountKey string) func(context.Context, ai.AccountSelection, ai.AccountObserve, bool) (ai.AssistantImages, error) {
		return func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
			tried = append(tried, *selection.AccountKey)
			return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
		}
	}
	// First model: no pin, so the provider's selected account serves it and becomes the pin.
	_, _ = router.ImagesRoutes(context.Background(), first, request, attempt("a"))
	if key, present := router.PeekAccountKey("p", "s1", "m1"); !present || key == nil || *key != "a" {
		t.Fatalf("a success must pin the model's account, got %v %v", key, present)
	}
	// Another model has no pin: the affinity is per model, not per session.
	if _, present := router.PeekAccountKey("p", "s1", "m2"); present {
		t.Fatal("affinity must not leak across models")
	}
	// A block on the pinned account moves that model to a sibling without touching the pin of
	// the model that has one.
	tried = nil
	store.block("p", keyOf("a"), "", time.Now().Add(time.Minute).UnixMilli())
	store.accounts = []ai.CredentialAccountInfo{
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", keyOf("b"), false),
	}
	_, _ = router.ImagesRoutes(context.Background(), second, request, attempt("b"))
	if key, present := router.PeekAccountKey("p", "s1", "m2"); !present || key == nil || *key != "b" {
		t.Fatalf("the second model must pin its own account, got %v %v", key, present)
	}
	// Clearing a provider drops every pin for it.
	router.ClearProvider("p")
	if _, present := router.PeekAccountKey("p", "s1", "m1"); present {
		t.Fatal("ClearProvider must drop the provider's affinity")
	}
}

func TestRouterExplicitAPIKeyBypasses(t *testing.T) {
	store := newFakeAccountStore(
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", keyOf("b"), false),
	)
	router := ai.NewAccountRouter(store)
	model := okModel("p", "m")
	var selections []ai.AccountSelection
	_, err := router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{APIKeySet: true}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
		selections = append(selections, selection)
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonError, ErrorMessage: "insufficient_quota"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(selections) != 1 || selections[0].AccountKeySet {
		t.Fatalf("an explicit key must bypass the pool, got %+v", selections)
	}
}

func TestRouterNoEligibleAccountError(t *testing.T) {
	store := newFakeAccountStore(accountInfo("openai-codex", keyOf("free-account"), true))
	store.put("openai-codex", keyOf("free-account"), ai.Credential{Type: ai.CredentialOAuth, Access: codexToken("free")})
	router := ai.NewAccountRouter(store)
	model := okModel("openai-codex", "gpt-5.6-sol")
	var tried []string
	_, err := router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
		tried = append(tried, *selection.AccountKey)
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
	})
	if err == nil {
		t.Fatal("an ineligible account must fail before any request")
	}
	if !strings.Contains(err.Error(), "No stored account meets the plan requirement") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tried) != 0 {
		t.Fatalf("no request may be sent for an ineligible account, tried %v", tried)
	}
}

func TestWithAccountKeyPresence(t *testing.T) {
	overrides := ai.WithAccountKey(ai.AuthResolutionOverrides{}, nil)
	if !overrides.AccountKeySet || overrides.AccountKey != nil {
		t.Fatalf("an explicit nil key must be preserved as present, got %+v", overrides)
	}
	// Applying a request scope never overrides a caller's explicit choice.
	named := ai.WithAccountKey(ai.AuthResolutionOverrides{}, keyOf("chosen"))
	ctx := ai.WithAccountRequest(context.Background(), ai.AccountRequest{AccountKey: nil, AccountKeySet: true})
	if merged := ai.ApplyAccountRequest(ctx, named); merged.AccountKey == nil || *merged.AccountKey != "chosen" {
		t.Fatalf("a caller's account must win, got %+v", merged)
	}
	if merged := ai.ApplyAccountRequest(ctx, ai.AuthResolutionOverrides{}); !merged.AccountKeySet || merged.AccountKey != nil {
		t.Fatalf("the request scope must supply a present nil key, got %+v", merged)
	}
}

// okStream builds a stream that starts and finishes with one text block.
func okStream() *ai.AssistantMessageEventStream {
	stream := ai.NewAssistantMessageEventStream()
	partial := &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "hello"}}}
	_ = stream.Push(ai.StartEvent{Partial: partial})
	_ = stream.Push(ai.TextDeltaEvent{ContentIndex: 0, Delta: "hello", Partial: partial})
	final := &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "hello"}}, StopReason: ai.StopReasonStop}
	_ = stream.Push(ai.DoneEvent{Reason: ai.StopReasonStop, Message: final})
	return stream
}

// emptyErrorStream builds a stream whose only outcome is an error with no content.
func emptyErrorStream(message string) *ai.AssistantMessageEventStream {
	stream := ai.NewAssistantMessageEventStream()
	_ = stream.Push(ai.ErrorEvent{Reason: ai.StopReasonError, Error: &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{}, StopReason: ai.StopReasonError, ErrorMessage: message}})
	return stream
}

// partialErrorStream builds a stream that emits content and then fails.
func partialErrorStream(message string) *ai.AssistantMessageEventStream {
	stream := ai.NewAssistantMessageEventStream()
	partial := &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "half"}}}
	_ = stream.Push(ai.StartEvent{Partial: partial})
	_ = stream.Push(ai.TextDeltaEvent{ContentIndex: 0, Delta: "half", Partial: partial})
	_ = stream.Push(ai.ErrorEvent{Reason: ai.StopReasonError, Error: &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "half"}}, StopReason: ai.StopReasonError, ErrorMessage: message}})
	return stream
}

// collectStream reads a stream to its end and returns the visible text and the terminal message.
func collectStream(t *testing.T, stream *ai.AssistantMessageEventStream) (string, *ai.AssistantMessage) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var text strings.Builder
	for event := range stream.Events(ctx) {
		if delta, ok := event.(ai.TextDeltaEvent); ok {
			text.WriteString(delta.Delta)
		}
	}
	return text.String(), stream.Result()
}

func TestStreamRoutesRotatesBeforeContent(t *testing.T) {
	store := newFakeAccountStore(
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", keyOf("b"), false),
	)
	router := ai.NewAccountRouter(store)
	model := okModel("p", "m")
	var tried []string
	stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
		tried = append(tried, *selection.AccountKey)
		if *selection.AccountKey == "a" {
			observe(ai.ProviderResponse{Status: 429, Headers: map[string]string{}})
			return emptyErrorStream("insufficient_quota"), nil
		}
		return okStream(), nil
	})
	text, message := collectStream(t, stream)
	if text != "hello" {
		t.Fatalf("expected the sibling's answer, got %q", text)
	}
	if message == nil || message.StopReason != ai.StopReasonStop {
		t.Fatalf("expected a successful result, got %+v", message)
	}
	if len(tried) != 2 {
		t.Fatalf("expected both accounts to be tried, got %v", tried)
	}
}

func TestStreamRoutesNeverReplaysAfterContent(t *testing.T) {
	store := newFakeAccountStore(
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", keyOf("b"), false),
	)
	router := ai.NewAccountRouter(store)
	model := okModel("p", "m")
	var tried []string
	stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
		tried = append(tried, *selection.AccountKey)
		return partialErrorStream("usage_limit_reached"), nil
	})
	text, message := collectStream(t, stream)
	if len(tried) != 1 {
		t.Fatalf("a failure after content must not rotate, tried %v", tried)
	}
	if text != "half" {
		t.Fatalf("the partial answer must be delivered exactly once, got %q", text)
	}
	if message == nil || message.StopReason != ai.StopReasonError {
		t.Fatalf("the failure must be reported, got %+v", message)
	}
}

func TestStreamRoutesRetriesSameAccountAfterForcedRefresh(t *testing.T) {
	store := newFakeAccountStore(accountInfo("openai-codex", keyOf("a"), true))
	store.put("openai-codex", keyOf("a"), ai.Credential{Type: ai.CredentialOAuth, Access: codexToken("pro")})
	router := ai.NewAccountRouter(store)
	model := okModel("openai-codex", "gpt-5")
	var forces []bool
	stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
		forces = append(forces, force)
		if !force {
			observe(ai.ProviderResponse{Status: 401, Headers: map[string]string{}})
			return emptyErrorStream("invalidated oauth token"), nil
		}
		return okStream(), nil
	})
	text, _ := collectStream(t, stream)
	if text != "hello" {
		t.Fatalf("the refreshed attempt must answer, got %q", text)
	}
	if len(forces) != 2 || forces[0] || !forces[1] {
		t.Fatalf("expected one plain then one forced attempt, got %v", forces)
	}
}

func TestStreamRoutesTransientFailureSurfaces(t *testing.T) {
	store := newFakeAccountStore(
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", keyOf("b"), false),
	)
	router := ai.NewAccountRouter(store)
	model := okModel("p", "m")
	var tried []string
	stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
		tried = append(tried, *selection.AccountKey)
		return emptyErrorStream("connection reset by peer"), nil
	})
	_, message := collectStream(t, stream)
	if len(tried) != 1 {
		t.Fatalf("a transport failure must not rotate, tried %v", tried)
	}
	if message == nil || message.StopReason != ai.StopReasonError || !strings.Contains(message.ErrorMessage, "connection reset") {
		t.Fatalf("expected the transport failure, got %+v", message)
	}
}

func TestRouterRotationEventCarriesNullIdentities(t *testing.T) {
	// Account "b" has no identity key, so the rotation must report a nil toAccountKey rather
	// than an empty string: an identity-less account is not a missing one.
	store := newFakeAccountStore(
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", nil, false),
	)
	router := ai.NewAccountRouter(store)
	var events []ai.AccountRotation
	router.OnRotation(func(event ai.AccountRotation) { events = append(events, event) })
	model := okModel("p", "m")
	_, _ = router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{SessionID: "s1"}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
		if selection.AccountKey == nil {
			return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
		}
		observe(ai.ProviderResponse{Status: 429, Headers: map[string]string{}})
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonError, ErrorMessage: "insufficient_quota"}, nil
	})
	if len(events) != 1 {
		t.Fatalf("expected exactly one rotation notice, got %d: %+v", len(events), events)
	}
	event := events[0]
	if event.ProviderID != "p" || event.Reason != "quota" || event.SessionID != "s1" {
		t.Fatalf("unexpected rotation: %+v", event)
	}
	if event.FromAccountKey == nil || *event.FromAccountKey != "a" {
		t.Fatalf("the rotation must name the rejected account, got %v", event.FromAccountKey)
	}
	if event.ToAccountKey != nil {
		t.Fatalf("an identity-less target must stay nil, got %q", *event.ToAccountKey)
	}
	if event.Message == "" {
		t.Fatal("the rotation message is what the status line shows")
	}
}

// hangingStream builds an attempt stream that starts, never terminates and never resolves a
// result: what a provider's iterator does when it closes on our cancellation. Its Events exits on
// the caller's cancellation while Result would stay pending forever (event_stream.go: End without
// a result leaves Result unresolved).
func hangingStream(withContent bool) *ai.AssistantMessageEventStream {
	stream := ai.NewAssistantMessageEventStream()
	partial := &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{}}
	if withContent {
		partial.Content = []ai.AssistantContentBlock{ai.TextContent{Text: "partial"}}
	}
	_ = stream.Push(ai.StartEvent{Partial: partial})
	if withContent {
		_ = stream.Push(ai.TextDeltaEvent{ContentIndex: 0, Delta: "partial", Partial: partial})
	}
	return stream
}

// assertStreamSettles fails when the routed stream has not resolved its result within a short
// bound, which is what a router waiting on a provider's unending result looks like.
func assertStreamSettles(t *testing.T, stream *ai.AssistantMessageEventStream) *ai.AssistantMessage {
	t.Helper()
	settled := make(chan *ai.AssistantMessage, 1)
	go func() {
		// A consumer iteration alongside Result is what a session does; both must finish.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for range stream.Events(ctx) {
		}
		settled <- stream.Result()
	}()
	select {
	case message := <-settled:
		return message
	case <-time.After(3 * time.Second):
		t.Fatal("the routed stream never settled: the router is waiting on a result that cannot resolve")
		return nil
	}
}

func TestStreamRoutesCancellationWithoutContentSettles(t *testing.T) {
	store := newFakeAccountStore(
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", keyOf("b"), false),
	)
	router := ai.NewAccountRouter(store)
	model := okModel("p", "m")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var tried int
	stream := router.StreamRoutes(ctx, model, ai.AccountRouteRequest{}, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
		tried++
		return hangingStream(false), nil
	})
	// Nothing terminal ever arrives: the consumer is cancelled before content.
	go func() {
		<-time.After(50 * time.Millisecond)
		cancel()
	}()
	message := assertStreamSettles(t, stream)
	if message == nil {
		t.Fatal("a cancelled routed stream must settle with a terminal message")
	}
	if message.StopReason != ai.StopReasonAborted {
		t.Fatalf("a cancellation must end aborted, got %q", message.StopReason)
	}
	if tried != 1 {
		t.Fatalf("a cancellation must not rotate accounts, tried %d", tried)
	}
}

func TestStreamRoutesCancellationAfterContentSettles(t *testing.T) {
	store := newFakeAccountStore(
		accountInfo("p", keyOf("a"), true),
		accountInfo("p", keyOf("b"), false),
	)
	router := ai.NewAccountRouter(store)
	model := okModel("p", "m")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var tried int
	stream := router.StreamRoutes(ctx, model, ai.AccountRouteRequest{}, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
		tried++
		return hangingStream(true), nil
	})
	go func() {
		<-time.After(50 * time.Millisecond)
		cancel()
	}()
	message := assertStreamSettles(t, stream)
	if message == nil || message.StopReason != ai.StopReasonAborted {
		t.Fatalf("a cancelled stream with partial content must end aborted, got %+v", message)
	}
	// Content already escaped, so a sibling account must never replay it.
	if tried != 1 {
		t.Fatalf("a failure after content must not rotate, tried %d", tried)
	}
	// The content the consumer already saw is preserved, not replaced by the abort.
	if len(message.Content) == 0 {
		t.Fatal("the aborted terminal must not discard the content the consumer already saw")
	}
	if text, ok := message.Content[0].(ai.TextContent); !ok || text.Text != "partial" {
		t.Fatalf("the aborted terminal must keep the emitted text, got %#v", message.Content)
	}
}

func TestStreamRoutesUnpooledWithoutResultSettles(t *testing.T) {
	// A single-account store never routes, yet the consumer must still settle when the provider
	// closes its iterator without resolving a result.
	router := ai.NewAccountRouter(nil)
	model := okModel("p", "m")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := router.StreamRoutes(ctx, model, ai.AccountRouteRequest{}, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
		return hangingStream(false), nil
	})
	go func() {
		<-time.After(50 * time.Millisecond)
		cancel()
	}()
	message := assertStreamSettles(t, stream)
	if message == nil || message.StopReason != ai.StopReasonAborted {
		t.Fatalf("an unpooled cancellation must settle aborted, got %+v", message)
	}
}

func TestStreamRoutesLiveProviderWithoutResultReportsError(t *testing.T) {
	// A provider that closes its iterator without a result while the request is still live is a
	// stream failure: the router must not call it a success and must not call it an abort.
	router := ai.NewAccountRouter(nil)
	model := okModel("p", "m")
	stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
		inner := hangingStream(false)
		inner.End()
		return inner, nil
	})
	message := assertStreamSettles(t, stream)
	if message == nil {
		t.Fatal("a malformed provider must still produce a terminal message")
	}
	if message.StopReason != ai.StopReasonError {
		t.Fatalf("a live provider without a result is a stream error, got %q", message.StopReason)
	}
	if message.ErrorMessage == "" {
		t.Fatal("the stream failure must say what happened")
	}
}
