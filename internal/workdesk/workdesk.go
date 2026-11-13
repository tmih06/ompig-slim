// Package workdesk hosts PiG Durable independently of the daily coding runtime.
// A state directory owns one immutable assignment and one SQLite task journal.
package workdesk

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/MichaelKinsy/PiG/durable/env"
	envnode "github.com/MichaelKinsy/PiG/durable/env/node"
	"github.com/MichaelKinsy/PiG/durable/harness"
	durablesqlite "github.com/MichaelKinsy/PiG/durable/storage/sqlite"
	sqlitenode "github.com/MichaelKinsy/PiG/durable/storage/sqlite/node"
	"github.com/MichaelKinsy/PiG/durable/tools"
	"github.com/gofrs/flock"
)

// assignment is the fsynced admission intent. Replaying its request ID bridges a
// crash between writing this file and admitting the input to the task journal.
// Version gates host policy changes independently of PiG's storage migrations.
type assignment struct {
	Version   int    `json:"version"`
	RequestID string `json:"request_id"`
	Prompt    string `json:"prompt"`
	CWD       string `json:"cwd"`
	Model     string `json:"model"`
	MaxTurns  int    `json:"max_turns"`
}

// Result is a committed submission receipt, not a claim that a PR is approved.
// Unanswered includes retry/turn-limit failures; pending needs a running host.
type Result struct {
	RequestID string        `json:"request_id"`
	Status    string        `json:"status"`
	Answer    string        `json:"answer,omitempty"`
	Reason    string        `json:"reason,omitempty"`
	Usage     []UsageReport `json:"usage,omitempty"`
	Events    []WorkerEvent `json:"events,omitempty"`
}

// WorkerEvent is a provider-neutral transcript event suitable for a daemon
// adapter. Payload fields are intentionally bounded to the event vocabulary
// Multica already persists: text, tool calls, and tool results.
type WorkerEvent struct {
	ID      string         `json:"id,omitempty"`
	Type    string         `json:"type"`
	CallID  string         `json:"call_id,omitempty"`
	Tool    string         `json:"tool,omitempty"`
	Content string         `json:"content,omitempty"`
	Input   map[string]any `json:"input,omitempty"`
	Output  string         `json:"output,omitempty"`
	Usage   *UsageReport   `json:"usage,omitempty"`
}

// UsageReport is the provider usage attached to one durable worker receipt.
// It intentionally uses Multica's daemon usage vocabulary so a factory can
// forward it without guessing provider-specific fields.
type UsageReport struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd,omitempty"`
}

// Run executes run/resume/status and writes one JSON receipt. Callers provide
// cancellation and handle errors; cancellation stops this host, not its durable
// assignment. Concurrent writers fail immediately; status never starts tasks.
func Run(ctx context.Context, args []string, out io.Writer) (runErr error) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			_, err := fmt.Fprintln(out, "usage: ompig-slim durable run --state DIR --cwd REPO --model PROVIDER/MODEL --request-id ID [--max-turns 100] [--events-file FILE] [--control-file FILE] PROMPT\n       ompig-slim durable resume --state DIR [--events-file FILE] [--control-file FILE]\n       ompig-slim durable status --state DIR")
			return err
		}
	}
	if len(args) == 0 {
		return errors.New("usage: durable run|resume|status --state DIR [options]")
	}
	command := args[0]
	if command != "run" && command != "resume" && command != "status" {
		return fmt.Errorf("unknown durable command %q", command)
	}
	flags := flag.NewFlagSet("durable "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	state := flags.String("state", "", "persistent workdesk directory (required)")
	eventsPath := flags.String("events-file", "", "append committed worker events as JSONL")
	controlPath := flags.String("control-file", "", "read supplemental prompts as JSONL")
	var job assignment
	if command == "run" {
		flags.StringVar(&job.CWD, "cwd", "", "repository directory (required)")
		flags.StringVar(&job.Model, "model", "", "provider/model (required)")
		flags.StringVar(&job.RequestID, "request-id", "", "stable assignment identity (required)")
		flags.IntVar(&job.MaxTurns, "max-turns", 100, "maximum tool rounds, retained across restart")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *state == "" {
		return errors.New("--state is required")
	}
	statePath, err := filepath.Abs(*state)
	if err != nil {
		return err
	}
	if command == "run" {
		job.Version = 1
		job.Prompt = strings.Join(flags.Args(), " ")
		if job.CWD == "" || job.Model == "" || strings.TrimSpace(job.RequestID) == "" || strings.TrimSpace(job.Prompt) == "" || job.MaxTurns < 1 {
			return errors.New("run requires --cwd, --model, --request-id, a prompt, and --max-turns >= 1")
		}
		if _, _, ok := strings.Cut(job.Model, "/"); !ok {
			return errors.New("--model must be provider/model")
		}
		job.CWD, err = filepath.Abs(job.CWD)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(statePath, 0700); err != nil {
			return err
		}
	} else if flags.NArg() != 0 {
		return errors.New("resume/status accept no prompt")
	}
	if command == "status" {
		job, err = loadAssignment(statePath)
		if err != nil {
			return err
		}
		result, err := status(ctx, statePath, job)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(result)
	}
	if err = os.Chmod(statePath, 0700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(statePath, "owner.lock"))
	owned, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !owned {
		return errors.New("workdesk already has an active owner")
	}
	defer func() { runErr = errors.Join(runErr, lock.Close()) }()
	saved, err := loadAssignment(statePath)
	if err == nil {
		if command == "run" && saved != job {
			return errors.New("state belongs to another assignment; use resume or a new state directory")
		}
		job = saved
	} else if command == "run" && errors.Is(err, os.ErrNotExist) {
		if err = saveAssignment(statePath, job); err != nil {
			return err
		}
	} else {
		return err
	}
	info, err := os.Stat(job.CWD)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("saved --cwd is not a directory")
	}
	models, err := coding.CreateModelRuntime(ctx, coding.CreateModelRuntimeOptions{})
	if err != nil {
		return err
	}
	defer models.Close()
	provider, id, _ := strings.Cut(job.Model, "/")
	if models.GetModel(provider, id) == nil {
		return fmt.Errorf("model %s is unavailable; restore its provider configuration before resuming", job.Model)
	}
	storage, err := openStorage(statePath)
	if err != nil {
		return err
	}
	registry := harness.CreateRegistry()
	if err = registry.Install(tools.CodingTools); err != nil {
		storage.Close(context.Background())
		return err
	}
	h, err := harness.OpenHarness(ctx, storage, harness.HarnessOptions{
		Models: models, Registry: registry,
		Identity: &ai.ClientIdentity{AttributionNull: true},
		Env: func(_ context.Context, _ harness.EnvTarget) (env.ExecutionEnv, error) {
			return envnode.NewNodeExecutionEnv(envnode.NodeExecutionEnvOptions{Cwd: job.CWD}), nil
		},
	})
	if err != nil {
		storage.Close(context.Background())
		return err
	}
	defer func() { runErr = errors.Join(runErr, h.Close(context.Background())) }()
	root, err := h.Root(ctx, &harness.RootOptions{Agent: &harness.AgentChange{
		Model: harness.SetTo(durable.ModelRef{Provider: provider, ModelId: id}), Cwd: harness.SetTo(job.CWD),
		MaxTurns:     harness.SetTo(float64(job.MaxTurns)),
		Instructions: harness.SetTo("Work autonomously on the assigned objective. Inspect the repository instructions, implement the change, run relevant checks, and inspect the diff. Do not ask for routine confirmations. Never claim checks passed unless run. If a tool was interrupted, inspect its effects before deciding whether to retry. Report blockers honestly. Ticket orchestration, push and PR policy belong to the caller."),
	}})
	if err != nil {
		return err
	}
	var eventWriter *workerEventWriter
	if *eventsPath != "" {
		eventWriter, err = newWorkerEventWriter(*eventsPath)
		if err != nil {
			return err
		}
		defer eventWriter.Close()
		observation, observeErr := h.ObserveConversation(ctx, root.Id(), eventWriter, nil)
		if observeErr != nil {
			return observeErr
		}
		defer observation.Detach()
	}
	if *controlPath != "" {
		go controlLoop(ctx, root, *controlPath)
	}
	submission, err := root.Submit(ctx, durable.SubmissionDraft{Type: durable.SubmissionTypeInput, Content: ai.UserText(job.Prompt), RequestId: &job.RequestID})
	if err != nil {
		return err
	}
	settled, err := submission.Wait(ctx)
	if err != nil {
		return err
	}
	if err = root.WaitForIdle(ctx); err != nil {
		return err
	}
	result, err := receipt(ctx, storage, settled, job)
	if err != nil {
		return err
	}
	if usage, usageErr := h.Usage(ctx); usageErr == nil {
		result.Usage = usageReportsFromState(usage)
	}
	if err = json.NewEncoder(out).Encode(result); err != nil {
		return err
	}
	if result.Status != "done" {
		return fmt.Errorf("workdesk ended %s: %s", result.Status, result.Reason)
	}
	return nil
}

type controlMessage struct {
	Content string `json:"content"`
}

func decodeControlMessages(data []byte) ([]controlMessage, int64) {
	last := strings.LastIndexByte(string(data), '\n')
	if last < 0 {
		return nil, 0
	}
	consumed := int64(last + 1)
	var messages []controlMessage
	for _, line := range strings.Split(string(data[:last]), "\n") {
		var message controlMessage
		if json.Unmarshal([]byte(line), &message) == nil && strings.TrimSpace(message.Content) != "" {
			messages = append(messages, message)
		}
	}
	return messages, consumed
}

func controlLoop(ctx context.Context, conversation harness.Conversation, path string) {
	var offset int64
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := os.ReadFile(path)
			if err != nil || int64(len(data)) <= offset {
				continue
			}
			messages, consumed := decodeControlMessages(data[offset:])
			for _, message := range messages {
				requestID := fmt.Sprintf("control-%d", time.Now().UnixNano())
				_, _ = conversation.Submit(ctx, durable.SubmissionDraft{Type: durable.SubmissionTypeInput, Content: ai.UserText(message.Content), RequestId: &requestID})
			}
			offset += consumed
		}
	}
}

// openStorage opens PiG's canonical SQLite journal with FULL synchronization.
// Power-loss guarantees still depend on the volume honoring SQLite's fsyncs.
func openStorage(state string) (*durablesqlite.SqliteStorage, error) {
	path := filepath.Join(state, "session.sqlite")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	err = errors.Join(file.Chmod(0600), file.Close())
	if err != nil {
		return nil, err
	}
	db, err := sqlitenode.OpenNodeSqliteDatabase(path, sqlitenode.NodeSqliteDatabaseOptions{})
	if err != nil {
		return nil, err
	}
	if err = db.Exec("PRAGMA synchronous = FULL"); err != nil {
		db.Close()
		return nil, err
	}
	store, err := durablesqlite.Open(db)
	if err != nil {
		db.Close()
	}
	return store, err
}

// loadAssignment refuses unknown host versions rather than silently changing
// the recovery policy of an existing journal.
func loadAssignment(state string) (assignment, error) {
	var job assignment
	raw, err := os.ReadFile(filepath.Join(state, "job.json"))
	if err != nil {
		return job, err
	}
	if err = json.Unmarshal(raw, &job); err != nil {
		return job, err
	}
	if job.Version != 1 {
		return job, fmt.Errorf("unsupported workdesk version %d", job.Version)
	}
	return job, nil
}

// saveAssignment publishes admission intent atomically and flushes both file and
// directory before any input can execute. A crash may leave an unused temp file.
func saveAssignment(state string, job assignment) error {
	raw, err := json.Marshal(job)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(state, ".job-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(file.Name(), filepath.Join(state, "job.json")); err != nil {
		return err
	}
	dir, err := os.Open(state)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// status reads committed records without opening a scheduler or using model
// credentials. An admission intent without a database is still pending work.
func status(ctx context.Context, state string, job assignment) (Result, error) {
	result := Result{RequestID: job.RequestID, Status: "pending"}
	if _, err := os.Stat(filepath.Join(state, "session.sqlite")); errors.Is(err, os.ErrNotExist) {
		return result, nil
	} else if err != nil {
		return result, err
	}
	storage, err := openStorage(state)
	if err != nil {
		return result, err
	}
	defer storage.Close(context.Background())
	page, err := storage.ScanSubmissions(ctx, durable.SubmissionQuery{}, 1, nil)
	if err != nil {
		return result, err
	}
	if len(page.Items) == 0 {
		return result, nil
	}
	return receipt(ctx, storage, page.Items[0], job)
}

type workerEventWriter struct {
	mu   sync.Mutex
	file *os.File
	seen map[durable.EntryId]struct{}
}

func newWorkerEventWriter(path string) (*workerEventWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	return &workerEventWriter{file: file, seen: make(map[durable.EntryId]struct{})}, nil
}

func (w *workerEventWriter) Advance(_ context.Context, call durable.WatchCall[harness.ConversationView]) {
	if call.Kind != durable.WatchCallValue {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, entry := range call.Value.Entries {
		if _, ok := w.seen[entry.Id]; ok {
			continue
		}
		w.seen[entry.Id] = struct{}{}
		for messageIndex, message := range entry.Model {
			for eventIndex, event := range workerEventsFromMessage(message) {
				event.ID = fmt.Sprintf("%d:%d:%d", entry.Id, messageIndex, eventIndex)
				if raw, err := json.Marshal(event); err == nil {
					_, _ = w.file.Write(append(raw, '\n'))
				}
			}
		}
	}
	_ = w.file.Sync()
}

func (w *workerEventWriter) End(durable.WatchEnd) {}

func (w *workerEventWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

func workerEventsFromMessage(message ai.Message) []WorkerEvent {
	switch value := message.(type) {
	case ai.AssistantMessage:
		events := make([]WorkerEvent, 0, len(value.Content))
		for _, block := range value.Content {
			switch block := block.(type) {
			case ai.TextContent:
				events = append(events, WorkerEvent{Type: "text", Content: block.Text})
			case ai.ToolCall:
				events = append(events, WorkerEvent{Type: "tool_call", CallID: block.ID, Tool: block.Name, Input: block.Arguments})
			}
		}
		if value.Provider != "" || value.Model != "" || value.Usage.TotalTokens > 0 {
			events = append(events, WorkerEvent{Type: "usage", Usage: &UsageReport{
				Provider: value.Provider, Model: value.Model,
				InputTokens: value.Usage.Input, OutputTokens: value.Usage.Output,
				CacheReadTokens: value.Usage.CacheRead, CacheWriteTokens: value.Usage.CacheWrite,
				CostUSD: value.Usage.Cost.Total,
			}})
		}
		return events
	case ai.ToolResultMessage:
		payload, _ := json.Marshal(value.Content)
		return []WorkerEvent{{Type: "tool_result", CallID: value.ToolCallID, Tool: value.ToolName, Output: string(payload)}}
	default:
		return nil
	}
}

func usageReportsFromState(state harness.UsageState) []UsageReport {
	reports := make([]UsageReport, 0, len(state.Models))
	for modelRef, usage := range state.Models {
		provider, model, _ := strings.Cut(modelRef, "/")
		reports = append(reports, UsageReport{
			Provider: provider, Model: model,
			InputTokens: usage.Input, OutputTokens: usage.Output,
			CacheReadTokens: usage.CacheRead, CacheWriteTokens: usage.CacheWrite,
			CostUSD: usage.Cost.Total,
		})
	}
	return reports
}

// receipt derives output from the canonical submission and immutable answer.
func receipt(ctx context.Context, storage durable.Storage, record durable.SubmissionRecord, job assignment) (Result, error) {
	result := Result{RequestID: job.RequestID, Status: string(record.Status)}
	if record.Reason != nil {
		result.Reason = *record.Reason
	}
	if record.Answer == nil {
		return result, nil
	}
	entry, err := storage.Entry(ctx, *record.Answer)
	if err != nil || entry == nil {
		return result, err
	}
	for messageIndex, message := range entry.Entry.Model {
		for eventIndex, event := range workerEventsFromMessage(message) {
			event.ID = fmt.Sprintf("%d:%d:%d", entry.Entry.Id, messageIndex, eventIndex)
			result.Events = append(result.Events, event)
		}
		assistant, ok := message.(ai.AssistantMessage)
		if !ok {
			continue
		}
		for _, block := range assistant.Content {
			if value, ok := block.(ai.TextContent); ok {
				result.Answer += value.Text
			}
		}
		if assistant.Provider != "" || assistant.Model != "" || assistant.Usage.TotalTokens > 0 {
			result.Usage = append(result.Usage, UsageReport{
				Provider: assistant.Provider, Model: assistant.Model,
				InputTokens: assistant.Usage.Input, OutputTokens: assistant.Usage.Output,
				CacheReadTokens: assistant.Usage.CacheRead, CacheWriteTokens: assistant.Usage.CacheWrite,
				CostUSD: assistant.Usage.Cost.Total,
			})
		}
	}
	return result, nil
}
