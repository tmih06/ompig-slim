package coding

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/codingagent/compaction"
)

type idleCompactionSnapshot struct {
	runSeq        uint64
	sessionID     string
	leafID        *string
	modelProvider string
	modelID       string
	entries       []icodingagent.SessionEntry
}

func (s *Session) idleCompactionPolicy() icodingagent.IdleCompactionPolicy {
	settings := s.SettingsManager()
	return icodingagent.IdleCompactionPolicy{
		Enabled:      settings.GetCompactionEnabled() && settings.GetCompactionIdleEnabled(),
		DelaySeconds: settings.GetCompactionIdleDelaySeconds(),
	}
}

// idleCompactionDeadline is the warmer's offer at a captured request's idle
// deadline. Claimed runs compaction inline on the warmer's task; Busy leaves
// the offer pending for the session's next idle signal; anything else
// declines for good.
func (s *Session) idleCompactionDeadline(ctx context.Context, request *icodingagent.CacheWarmRequest) icodingagent.IdleOfferResult {
	if ctx.Err() != nil || request == nil || request.Model == nil {
		return icodingagent.IdleOfferDeclined
	}
	settings := s.SettingsManager()
	if !settings.GetCompactionEnabled() || !settings.GetCompactionIdleEnabled() {
		return icodingagent.IdleOfferDeclined
	}
	s.mu.Lock()
	if s.inner == nil || s.inner.ID() != request.Options.SessionID {
		s.mu.Unlock()
		return icodingagent.IdleOfferDeclined
	}
	model := s.Model()
	if model == nil || providerID(model) != providerID(request.Model) || model.ID != request.Model.ID {
		s.mu.Unlock()
		return icodingagent.IdleOfferDeclined
	}
	compactionModel, modelErr := s.compactionModel(model)
	if modelErr != nil || compactionModel == nil ||
		providerID(compactionModel) != providerID(request.Model) || compactionModel.ID != request.Model.ID {
		s.mu.Unlock()
		return icodingagent.IdleOfferDeclined
	}
	compactionSettings, settingsErr := s.compactionSettings()
	if settingsErr != nil || !compactionSettings.Enabled {
		s.mu.Unlock()
		return icodingagent.IdleOfferDeclined
	}
	snapshot := idleCompactionSnapshot{
		sessionID:     s.inner.ID(),
		leafID:        s.inner.GetLeafID(),
		modelProvider: providerID(model),
		modelID:       model.ID,
		entries:       s.currentBranch(),
	}
	s.mu.Unlock()

	prep := compaction.PrepareCompaction(snapshot.entries, compactionSettings)
	if prep == nil {
		return icodingagent.IdleOfferDeclined
	}
	if outcome := s.claimIdleCompaction(request, &snapshot); outcome != icodingagent.IdleOfferClaimed {
		return outcome
	}
	s.runIdleCompaction(ctx, request, compactionModel, *prep, snapshot)
	return icodingagent.IdleOfferClaimed
}

// claimIdleCompaction takes the compaction slot for an idle offer that still
// matches the session, or reports Busy when work that can change the context
// is settling or pending. Lock order matches run admission: pendingBashMu,
// runState.mu, compactMu.
func (s *Session) claimIdleCompaction(request *icodingagent.CacheWarmRequest, snapshot *idleCompactionSnapshot) icodingagent.IdleOfferResult {
	s.pendingBashMu.Lock()
	defer s.pendingBashMu.Unlock()
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	snapshot.runSeq = s.runState.generation
	busy := s.runState.active.Load() || s.settlementBusyLocked() ||
		s.runState.userRunsPending.Load() > 0 || s.agent.HasQueuedMessages() ||
		len(s.runState.pendingCustom) > 0 || len(s.pendingBashMessages) > 0
	if busy {
		return icodingagent.IdleOfferBusy
	}
	if request.IdleEligible != nil && !request.IdleEligible() {
		return icodingagent.IdleOfferDeclined
	}
	s.compactMu.Lock()
	defer s.compactMu.Unlock()
	if s.compacting.Load() || s.branchSumCancel != nil {
		return icodingagent.IdleOfferBusy
	}
	s.compacting.Store(true)
	s.idleCompacting = true
	s.compactDone = make(chan struct{})
	s.compactAbortRequested = false
	return icodingagent.IdleOfferClaimed
}

// The caller holds runState.mu.
func (s *Session) requestIdleCompactionAbortLocked() {
	s.compactMu.Lock()
	if s.idleCompacting {
		s.compactAbortRequested = true
		if cancel := s.compactCancel; cancel != nil {
			cancel()
		}
	}
	s.compactMu.Unlock()
}

// CancelIdleCompaction preempts an idle compaction for user work. It only
// signals: the abort flag and generation fence make the in-flight compaction
// drop its commit, so callers never wait on a provider abort. It returns
// false only while a non-idle compaction or tree navigation owns the slot.
func (s *Session) CancelIdleCompaction(context.Context) bool {
	s.compactMu.Lock()
	defer s.compactMu.Unlock()
	if s.idleCompacting && s.compacting.Load() {
		s.compactAbortRequested = true
		if cancel := s.compactCancel; cancel != nil {
			cancel()
		}
		return true
	}
	return !s.compacting.Load() && s.branchSumCancel == nil
}

// waitIdleCompactionRelease waits for a finishing idle compaction to free the
// slot, for a caller that claims the slot itself. It reports false when a
// non-idle operation owns or takes the slot first.
func (s *Session) waitIdleCompactionRelease(ctx context.Context) bool {
	for {
		s.compactMu.Lock()
		if s.idleCompacting && s.compacting.Load() {
			done := s.compactDone
			s.compactMu.Unlock()
			if done == nil {
				continue
			}
			select {
			case <-ctx.Done():
				return false
			case <-done:
			}
			continue
		}
		free := !s.compacting.Load() && s.branchSumCancel == nil
		s.compactMu.Unlock()
		return free
	}
}

// runIdleCompaction summarizes and commits an idle compaction. The claim is
// released before end events so listeners can submit prompts; the start event
// is queued without waiting, so a listener that preempts or prompts from the
// event cannot deadlock the claim.
func (s *Session) runIdleCompaction(ctx context.Context, request *icodingagent.CacheWarmRequest, model *ai.Model, prep compaction.CompactionPreparation, snapshot idleCompactionSnapshot) {
	compactCtx, cancel := context.WithCancel(ctx)
	s.compactMu.Lock()
	s.compactCancel = cancel
	s.compactCancelSeq++
	cancelSeq := s.compactCancelSeq
	abortRequested := s.compactAbortRequested
	s.compactMu.Unlock()
	if abortRequested {
		cancel()
	}
	finish := sync.OnceFunc(func() {
		cancel()
		s.compactMu.Lock()
		if s.compactCancelSeq == cancelSeq {
			s.compactCancel = nil
		}
		s.compactMu.Unlock()
		s.finishCompaction()
	})
	defer finish()

	fromExtension := false
	fail := func(err error, aborted bool) {
		errorMessage := ""
		if !aborted {
			errorMessage = "Idle compaction failed: " + err.Error()
		}
		finish()
		s.emitOrderedEventSync(agent.CompactionEndEvent{Reason: "idle", Aborted: aborted, ErrorMessage: errorMessage})
		s.emitSessionCompactFailed(ctx, extension.SessionCompactFailedEvent{Reason: "idle", ErrorMessage: errorMessage, Aborted: aborted, FromExtension: fromExtension})
	}

	s.emitOrderedEvent(agent.CompactionStartEvent{Reason: "idle", Model: compactionModelLabel(model)})
	if compactCtx.Err() != nil {
		fail(errCompactionCancelled, true)
		return
	}

	result, fromExt, err := s.extensionCompaction(compactCtx, &prep, snapshot.entries, "", "idle", false)
	fromExtension = fromExt
	cancelledByExtension := isCompactionCancelled(err)
	if err == nil && result == nil {
		var summary string
		var usage *ai.Usage
		summary, usage, err = s.idleSummarize(compactCtx, request)
		if err == nil && summary == "" {
			err = errors.New("provider returned an empty summary")
		}
		if err == nil {
			readFiles, modifiedFiles := compaction.ComputeFileLists(prep.FileOps)
			result = &sessionCompactionData{
				Summary:          summary,
				FirstKeptEntryID: prep.FirstKeptEntryID,
				TokensBefore:     prep.TokensBefore,
				Usage:            usage,
				Details:          compaction.CompactionDetails{ReadFiles: readFiles, ModifiedFiles: modifiedFiles},
			}
		}
	}
	aborted := compactCtx.Err() != nil || cancelledByExtension
	if err == nil && compactCtx.Err() != nil {
		err = errCompactionCancelled
	}
	if err != nil {
		fail(err, aborted)
		return
	}

	entryID, persistErr := s.commitIdleCompaction(compactCtx, request, result, fromExtension, snapshot)
	finish()
	if persistErr != nil {
		errorMessage := "Idle compaction failed: " + persistErr.Error()
		s.emitOrderedEventSync(agent.CompactionEndEvent{Reason: "idle", ErrorMessage: errorMessage})
		s.emitSessionCompactFailed(ctx, extension.SessionCompactFailedEvent{Reason: "idle", ErrorMessage: errorMessage, FromExtension: fromExtension})
		return
	}
	if entryID == "" {
		s.emitOrderedEventSync(agent.CompactionEndEvent{Reason: "idle", Aborted: true})
		s.emitSessionCompactFailed(ctx, extension.SessionCompactFailedEvent{Reason: "idle", Aborted: true, FromExtension: fromExtension})
		return
	}
	var estimatedTokensAfter int
	var entry icodingagent.SessionEntry
	var haveEntry bool
	s.mu.Lock()
	estimatedTokensAfter = estimateMessagesTokens(s.agent.MessagesSnapshot())
	entry, haveEntry = s.inner.GetEntry(entryID)
	s.mu.Unlock()
	if haveEntry {
		s.emitSessionCompact(ctx, entry, fromExtension, "idle", false)
	}
	s.emitCompactionEvent(agent.CompactionEndEvent{
		Reason:               "idle",
		Summary:              result.Summary,
		FirstKeptEntryID:     result.FirstKeptEntryID,
		TokensBefore:         result.TokensBefore,
		EstimatedTokensAfter: estimatedTokensAfter,
		Usage:                result.Usage,
		Details:              result.Details,
	})
}

// commitIdleCompaction persists the summary only while the request context is
// still exactly the messages the summary covered: same run, same leaf, same
// model, and the captured prefix plus only its successful response.
func (s *Session) commitIdleCompaction(ctx context.Context, request *icodingagent.CacheWarmRequest, result *sessionCompactionData, fromExtension bool, snapshot idleCompactionSnapshot) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	s.compactMu.Lock()
	abortRequested := s.compactAbortRequested
	s.compactMu.Unlock()
	if abortRequested || ctx.Err() != nil || s.runState.generation != snapshot.runSeq ||
		s.inner.ID() != snapshot.sessionID || !sameLeafID(s.inner.GetLeafID(), snapshot.leafID) {
		return "", nil
	}
	if model := s.Model(); model == nil || providerID(model) != snapshot.modelProvider || model.ID != snapshot.modelID {
		return "", nil
	}
	if request.IdleEligible != nil && !request.IdleEligible() {
		return "", nil
	}
	entryID, err := s.inner.AppendCompaction(result.Summary, result.FirstKeptEntryID, result.TokensBefore, result.Details, fromExtension, result.Usage)
	if err != nil {
		return "", err
	}
	s.refreshContext()
	return entryID, nil
}

func sameLeafID(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (s *Session) idleSummarize(ctx context.Context, request *icodingagent.CacheWarmRequest) (string, *ai.Usage, error) {
	transcript := s.idleCompactionTranscript(request)
	// Anthropic derives cache-keyed thinking budgets from maxTokens.
	completer := compaction.ProviderCompleter{StreamFn: s.streamWarmRequest}
	return completer.CompleteSimpleTranscript(ctx, request.Model, transcript, request.Options)
}

func (s *Session) idleCompactionTranscript(request *icodingagent.CacheWarmRequest) ai.TranscriptContext {
	messages := request.Context.Messages()
	// The offer goroutine runs off the owner loop, so it reads the lock-guarded snapshot.
	if last := lastAssistantMessage(s.agent.MessagesSnapshot()); last != nil && last.StopReason == ai.StopReasonStop {
		duplicate := false
		if n := len(messages); n > 0 {
			if tail, ok := messages[n-1].(ai.AssistantMessage); ok {
				duplicate = tail.Timestamp == last.Timestamp && tail.ResponseID == last.ResponseID
			}
		}
		if !duplicate {
			messages = append(messages, last.LLMMessage())
		}
	}
	messages = append(messages, ai.UserMessage{
		Content: ai.UserContentBlocks{ai.TextContent{Text: compaction.SUMMARIZATION_PROMPT +
			"\n\nInclude the newest messages in the summary as well; the kept tail they belong to stays in context verbatim."}},
		Timestamp: time.Now().UnixMilli(),
	})
	return ai.NormalizeContext(ai.Context{Messages: messages})
}
