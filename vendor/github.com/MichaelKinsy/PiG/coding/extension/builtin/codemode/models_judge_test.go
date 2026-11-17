package codemode

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

type judgeBridgeModels struct {
	called  int
	request ai.ClassifierContext
}

func (models *judgeBridgeModels) GetModelsOfType(ai.ModelType, ...string) []ai.AnyModel { return nil }
func (models *judgeBridgeModels) GetAvailableOfType(context.Context, ai.ModelType, ...string) ([]ai.AnyModel, error) {
	return nil, nil
}
func (models *judgeBridgeModels) GetModelOfType(ai.ModelType, string, string) ai.AnyModel { return nil }
func (models *judgeBridgeModels) GenerateImages(context.Context, *ai.ImageModel, ai.ImagesContext, ...ai.ModelsImagesOptions) ai.AssistantImages {
	return ai.AssistantImages{}
}
func (models *judgeBridgeModels) Judge(_ context.Context, request ai.ClassifierContext) ai.JudgeResult {
	models.called++
	models.request = request
	probability := 0.75
	usage := &ai.Usage{Input: 4, Output: 2, TotalTokens: 6}
	return ai.JudgeResult{
		Provider: "fixture", Model: "judge", Answers: map[string]ai.JudgeAnswer{
			"check": ai.JudgeBoolAnswer{Value: true, Probability: &probability},
		},
		Usage: usage, StopReason: ai.JudgeStopReasonStop,
	}
}

func TestModelsJudgeBridgeDelegatesAndTracksUsage(t *testing.T) {
	models := &judgeBridgeModels{}
	r := &run{}
	var toolName string
	var execute func(context.Context, json.RawMessage) (json.RawMessage, error)
	for _, tool := range r.modelGlobals(models) {
		if tool.Name == "models.judge" {
			toolName, execute = tool.Name, tool.Execute
		}
		if tool.Name == "models.classify" {
			t.Fatal("legacy models.classify is still exposed")
		}
	}
	if execute == nil || toolName != "models.judge" {
		t.Fatal("models.judge is not exposed")
	}
	contextJSON := `{"state":{"answer":4},"questions":{"check":{"type":"bool","instructions":"Is it correct?","criteria":{"true":"Yes","false":"No"}}}}`
	result, err := execute(context.Background(), json.RawMessage("["+contextJSON+"]"))
	if err != nil {
		t.Fatal(err)
	}
	if models.called != 1 || models.request.State["answer"] != float64(4) || len(models.request.Questions) != 1 {
		t.Fatalf("judge received the wrong context: called=%d request=%+v", models.called, models.request)
	}
	var wire map[string]any
	if err := json.Unmarshal(result, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["provider"] != "fixture" || wire["model"] != "judge" || wire["stopReason"] != "stop" {
		t.Fatalf("bridge result: %s", result)
	}
	answer := wire["answers"].(map[string]any)["check"].(map[string]any)
	if answer["type"] != "bool" || answer["value"] != true || answer["classifier"].(map[string]any)["probability"] != 0.75 {
		t.Fatalf("bridge answer: %s", result)
	}
	if r.modelUsage == nil || r.modelUsage.Input != 4 || r.modelUsage.Output != 2 || len(r.snapshot()) != 1 || r.snapshot()[0].Name != "models.judge" || r.snapshot()[0].Args != "@judge" || r.snapshot()[0].Status != StatusOK {
		t.Fatalf("nested call or usage was not recorded: calls=%+v usage=%+v", r.snapshot(), r.modelUsage)
	}
}

type concurrentJudgeBridgeModels struct {
	active     atomic.Int32
	maximum    atomic.Int32
	calls      atomic.Int32
	imageCalls atomic.Int32
	image      *ai.ImageModel
	started    chan struct{}
	release    <-chan struct{}
}

func (models *concurrentJudgeBridgeModels) GetModelsOfType(ai.ModelType, ...string) []ai.AnyModel {
	return nil
}
func (models *concurrentJudgeBridgeModels) GetAvailableOfType(context.Context, ai.ModelType, ...string) ([]ai.AnyModel, error) {
	return nil, nil
}
func (models *concurrentJudgeBridgeModels) GetModelOfType(modelType ai.ModelType, provider, id string) ai.AnyModel {
	if modelType == ai.ModelTypeImage && models.image != nil && models.image.Provider == provider && models.image.ID == id {
		return models.image
	}
	return nil
}
func (models *concurrentJudgeBridgeModels) GenerateImages(context.Context, *ai.ImageModel, ai.ImagesContext, ...ai.ModelsImagesOptions) ai.AssistantImages {
	models.imageCalls.Add(1)
	return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}
}
func (models *concurrentJudgeBridgeModels) Judge(context.Context, ai.ClassifierContext) ai.JudgeResult {
	active := models.active.Add(1)
	for maximum := models.maximum.Load(); active > maximum && !models.maximum.CompareAndSwap(maximum, active); maximum = models.maximum.Load() {
	}
	models.calls.Add(1)
	models.started <- struct{}{}
	<-models.release
	models.active.Add(-1)
	return ai.JudgeResult{Answers: map[string]ai.JudgeAnswer{"check": ai.JudgeBoolAnswer{Value: true}}, StopReason: ai.JudgeStopReasonStop}
}

func TestModelsJudgeBridgeLimitsConcurrentCallsAndSettlesAll(t *testing.T) {
	const calls = 9
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	models := &concurrentJudgeBridgeModels{started: make(chan struct{}, calls), release: release}
	r := &run{}
	var execute func(context.Context, json.RawMessage) (json.RawMessage, error)
	for _, tool := range r.modelGlobals(models) {
		if tool.Name == "models.judge" {
			execute = tool.Execute
			break
		}
	}
	if execute == nil {
		t.Fatal("models.judge is not exposed")
	}
	args := json.RawMessage(`[{"state":{},"questions":{"check":{"type":"bool","instructions":"Check","criteria":{"true":"Yes","false":"No"}}}}]`)
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(calls)
	results := make(chan error, calls)
	for range calls {
		go func() {
			ready.Done()
			<-start
			result, err := execute(context.Background(), args)
			if err == nil && (!json.Valid(result) || !strings.Contains(string(result), `"stopReason":"stop"`)) {
				err = errors.New("judge call did not settle successfully")
			}
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	for i := 0; i < maxConcurrentModelCalls; i++ {
		select {
		case <-models.started:
		case <-time.After(5 * time.Second):
			t.Fatal("judge calls did not fill the concurrency limit")
		}
	}
	select {
	case <-models.started:
		t.Fatal("a fifth judge call started before a slot was released")
	case <-time.After(100 * time.Millisecond):
	}
	if models.maximum.Load() > maxConcurrentModelCalls || models.active.Load() != maxConcurrentModelCalls {
		t.Fatalf("observed judge concurrency: max=%d active=%d", models.maximum.Load(), models.active.Load())
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < calls; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Errorf("judge call failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("queued judge calls did not settle")
		}
	}
	if models.calls.Load() != calls || models.active.Load() != 0 || models.maximum.Load() > maxConcurrentModelCalls {
		t.Fatalf("judge calls did not drain: calls=%d active=%d max=%d", models.calls.Load(), models.active.Load(), models.maximum.Load())
	}
	rows := r.snapshot()
	if len(rows) != calls {
		t.Fatalf("nested call rows=%d, want %d", len(rows), calls)
	}
	for _, row := range rows {
		if row.Status != StatusOK {
			t.Errorf("judge call did not settle: %+v", row)
		}
	}
}

func TestLimiterCancellationDoesNotStartQueuedModelCall(t *testing.T) {
	limit := &limiter{limit: 1}
	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan bool, 1)
	go func() {
		firstDone <- limit.run(context.Background(), func() {
			close(started)
			<-release
		})
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	secondRan := false
	secondDone := make(chan bool, 1)
	go func() { secondDone <- limit.run(ctx, func() { secondRan = true }) }()
	deadline := time.After(5 * time.Second)
	for {
		limit.mu.Lock()
		queued := len(limit.waiting) == 1
		limit.mu.Unlock()
		if queued {
			break
		}
		select {
		case <-deadline:
			t.Fatal("second model call did not queue")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	if <-secondDone {
		t.Fatal("cancelled queued call started")
	}
	close(release)
	if !<-firstDone || secondRan {
		t.Fatalf("limiter result: secondRan=%v", secondRan)
	}
	limit.mu.Lock()
	defer limit.mu.Unlock()
	if limit.active != 0 || len(limit.waiting) != 0 {
		t.Fatalf("limiter retained state after cancellation: active=%d waiting=%d", limit.active, len(limit.waiting))
	}
}

type queuedModelCallContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (ctx *queuedModelCallContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}

func TestModelsGenerateImagesQueuedCancellationReturnsAbortedResult(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	models := &concurrentJudgeBridgeModels{
		image:   &ai.ImageModel{ID: "fixture-image", API: "fixture-images", Provider: "fixture"},
		started: make(chan struct{}, maxConcurrentModelCalls),
		release: release,
	}
	r := &run{}
	var judgeExecute, imageExecute func(context.Context, json.RawMessage) (json.RawMessage, error)
	for _, tool := range r.modelGlobals(models) {
		switch tool.Name {
		case "models.judge":
			judgeExecute = tool.Execute
		case "models.generateImages":
			imageExecute = tool.Execute
		}
	}
	if judgeExecute == nil || imageExecute == nil {
		t.Fatal("model operations are not exposed")
	}
	judgeArgs := json.RawMessage(`[{"state":{},"questions":{"check":{"type":"bool","instructions":"Check","criteria":{"true":"Yes","false":"No"}}}}]`)
	judgeResults := make(chan error, maxConcurrentModelCalls)
	for range maxConcurrentModelCalls {
		go func() {
			_, err := judgeExecute(context.Background(), judgeArgs)
			judgeResults <- err
		}()
	}
	for range maxConcurrentModelCalls {
		select {
		case <-models.started:
		case <-time.After(5 * time.Second):
			t.Fatal("judge calls did not fill the model-call limit")
		}
	}

	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &queuedModelCallContext{Context: parent, waiting: make(chan struct{})}
	imageArgs := json.RawMessage(`[{"provider":"fixture","id":"fixture-image"},{"input":[{"type":"text","text":"draw"}]}]`)
	type imageResult struct {
		data json.RawMessage
		err  error
	}
	imageDone := make(chan imageResult, 1)
	go func() {
		data, err := imageExecute(ctx, imageArgs)
		imageDone <- imageResult{data: data, err: err}
	}()
	select {
	case <-ctx.waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("image call did not queue behind the active judge calls")
	}
	cancel()
	var output imageResult
	select {
	case output = <-imageDone:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled queued image call did not settle")
	}
	if output.err != nil {
		t.Fatal(output.err)
	}
	var generated ai.AssistantImages
	if err := json.Unmarshal(output.data, &generated); err != nil {
		t.Fatalf("decode image result %s: %v", output.data, err)
	}
	if generated.API != models.image.API || generated.Provider != models.image.Provider || generated.Model != models.image.ID || generated.StopReason != ai.ImagesStopReasonAborted || generated.ErrorMessage != context.Canceled.Error() || generated.Output == nil || len(generated.Output) != 0 || generated.Timestamp == 0 {
		t.Fatalf("queued cancellation returned an invalid image result: %+v", generated)
	}
	if models.imageCalls.Load() != 0 {
		t.Fatalf("cancelled image call reached the provider: %d calls", models.imageCalls.Load())
	}

	releaseOnce.Do(func() { close(release) })
	for range maxConcurrentModelCalls {
		select {
		case err := <-judgeResults:
			if err != nil {
				t.Errorf("judge call failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("active judge calls did not settle")
		}
	}
	rows := r.snapshot()
	if len(rows) != maxConcurrentModelCalls+1 || rows[len(rows)-1].Status != StatusCancelled {
		t.Fatalf("queued image call was not recorded as cancelled: %+v", rows)
	}
}

func TestModelsJudgeRejectsSelectorsAndExtraArguments(t *testing.T) {
	models := &judgeBridgeModels{}
	var execute func(context.Context, json.RawMessage) (json.RawMessage, error)
	for _, tool := range (&run{}).modelGlobals(models) {
		if tool.Name == "models.judge" {
			execute = tool.Execute
			break
		}
	}
	if execute == nil {
		t.Fatal("models.judge is not exposed")
	}
	valid := `{"state":{},"questions":{"check":{"type":"bool","instructions":"Check","criteria":{"true":"Yes","false":"No"}}}}`
	for _, selector := range []string{`"model":"google/gemini-decision-1"`, `"role":"judge"`} {
		contextJSON := strings.TrimSuffix(valid, "}") + "," + selector + "}"
		_, err := execute(context.Background(), json.RawMessage("["+contextJSON+"]"))
		if err == nil || !strings.Contains(err.Error(), "model and role selectors are configured outside the script") {
			t.Fatalf("selector %s was not rejected: %v", selector, err)
		}
	}
	_, err := execute(context.Background(), json.RawMessage("["+valid+",{}]"))
	if err == nil || !strings.Contains(err.Error(), "expects exactly one context argument") {
		t.Fatalf("extra argument was not rejected: %v", err)
	}
	for _, invalid := range []string{
		`{"state":{},"questions":{}}`,
		`{"state":{},"questions":{"check":{"type":"rating","instructions":"Check","criteria":[]}}}`,
	} {
		if _, err := execute(context.Background(), json.RawMessage("["+invalid+"]")); err == nil {
			t.Fatalf("invalid judge context reached the runtime: %s", invalid)
		}
	}
	if models.called != 0 {
		t.Fatalf("invalid bridge arguments reached the judge: %d calls", models.called)
	}
}
