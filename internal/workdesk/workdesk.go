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
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Answer    string `json:"answer,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Run executes run/resume/status and writes one JSON receipt. Callers provide
// cancellation and handle errors; cancellation stops this host, not its durable
// assignment. Concurrent writers fail immediately; status never starts tasks.
func Run(ctx context.Context, args []string, out io.Writer) (runErr error) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			_, err := fmt.Fprintln(out, "usage: ompig-slim durable run --state DIR --cwd REPO --model PROVIDER/MODEL --request-id ID [--max-turns 100] PROMPT\n       ompig-slim durable resume|status --state DIR")
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
		Env: func(_ context.Context, target harness.EnvTarget) (env.ExecutionEnv, error) {
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
	if err = json.NewEncoder(out).Encode(result); err != nil {
		return err
	}
	if result.Status != "done" {
		return fmt.Errorf("workdesk ended %s: %s", result.Status, result.Reason)
	}
	return nil
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

// receipt derives output from the canonical submission and immutable answer.
func receipt(ctx context.Context, storage durable.Storage, record durable.SubmissionRecord, job assignment) (Result, error) {
	result := Result{RequestID: job.RequestID, Status: string(record.Status)}
	if record.Reason != nil {
		result.Reason = *record.Reason
	}
	if record.Answer != nil {
		entry, err := storage.Entry(ctx, *record.Answer)
		if err != nil {
			return result, err
		}
		if entry != nil {
			for _, message := range entry.Entry.Model {
				if m, ok := message.(ai.AssistantMessage); ok {
					for _, block := range m.Content {
						if text, ok := block.(ai.TextContent); ok {
							result.Answer += text.Text
						}
					}
				}
			}
		}
	}
	return result, nil
}
