package subagents

import (
	"bufio"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/codemode"
)

//go:embed workflow.js
var workflowPrelude string

var workflowIDPattern = regexp.MustCompile(`^wf_[a-z0-9-]{6,}$`)

type WorkflowParams struct {
	Script     string          `json:"script"`
	ScriptPath string          `json:"scriptPath"`
	Name       string          `json:"name"`
	Args       json.RawMessage `json:"args,omitempty"`
	ResumeFrom string          `json:"resumeFromRunId,omitempty"`
}
type JournalEntry struct {
	Index   int             `json:"index"`
	Key     string          `json:"key"`
	Value   json.RawMessage `json:"value"`
	OK      bool            `json:"ok"`
	Resumed bool            `json:"resumed,omitempty"`
}
type Workflow struct {
	ID             string            `json:"id"`
	Meta           WorkflowMeta      `json:"meta"`
	Status         string            `json:"status"`
	ScriptPath     string            `json:"scriptPath"`
	StartedAt      int64             `json:"startedAt"`
	CompletedAt    int64             `json:"completedAt,omitempty"`
	DoneCount      int               `json:"doneCount"`
	TotalCount     int               `json:"totalCount"`
	Tokens         int64             `json:"tokens"`
	OutputTokens   int64             `json:"outputTokens"`
	Result         json.RawMessage   `json:"result,omitempty"`
	Error          string            `json:"error,omitempty"`
	Progress       []json.RawMessage `json:"progress"`
	Journal        []JournalEntry    `json:"journal"`
	Args           json.RawMessage   `json:"args,omitempty"`
	cancel         context.CancelFunc
	done           chan struct{}
	labels         map[string]string
	sem            chan struct{}
	journalIndex   int
	replay         []JournalEntry
	replayLive     bool
	nested         int
	consumed       bool
	pause          chan struct{}
	journalWriteMu *sync.Mutex
}

func (w Workflow) Active() bool { return w.Status == "running" || w.Status == "paused" }
func (w Workflow) ResultText() string {
	return fmt.Sprintf("Workflow %s (%s): %s\n%s\n%s", w.ID, w.Meta.Name, w.Status, string(w.Result), w.Error)
}
func copyWorkflow(w *Workflow) Workflow {
	out := *w
	out.Progress = slices.Clone(w.Progress)
	out.Journal = slices.Clone(w.Journal)
	out.Result = slices.Clone(w.Result)
	out.Args = slices.Clone(w.Args)
	out.labels = nil
	out.replay = nil
	out.sem = nil
	return out
}
func (m *Manager) Workflows() []Workflow {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Workflow, len(m.workflows))
	for i, w := range m.workflows {
		out[i] = copyWorkflow(w)
	}
	return out
}
func (m *Manager) Workflow(id string) (Workflow, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.workflows {
		if w.ID == id {
			return copyWorkflow(w), true
		}
	}
	return Workflow{}, false
}
func (m *Manager) StopWorkflow(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.workflows {
		if w.ID == id {
			w.cancel()
			return nil
		}
	}
	return fmt.Errorf("workflow not found: %s", id)
}

func (m *Manager) WaitWorkflow(ctx context.Context, id string) (Workflow, error) {
	m.mu.Lock()
	var target *Workflow
	for _, w := range m.workflows {
		if w.ID == id {
			target = w
			break
		}
	}
	if target == nil {
		m.mu.Unlock()
		return Workflow{}, fmt.Errorf("workflow not found: %s", id)
	}
	done := target.done
	m.mu.Unlock()
	select {
	case <-ctx.Done():
		return Workflow{}, ctx.Err()
	case <-done:
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	target.consumed = true
	return copyWorkflow(target), nil
}

func (m *Manager) PauseWorkflow(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.workflows {
		if w.ID == id {
			if !w.Active() {
				return fmt.Errorf("workflow has finished")
			}
			if w.Status == "paused" {
				close(w.pause)
				w.pause = nil
				w.Status = "running"
			} else {
				w.pause = make(chan struct{})
				w.Status = "paused"
			}
			return nil
		}
	}
	return fmt.Errorf("workflow not found")
}
func (m *Manager) WorkflowRecords(id string) []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Record
	for _, r := range m.runs {
		if r.WorkflowID == id && !r.retry {
			out = append(out, cloneRecord(r.Record))
		}
	}
	return out
}
func (m *Manager) SkipWorkflowAgent(workflow, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		if r.ID == id && r.WorkflowID == workflow {
			if !r.Active() {
				return fmt.Errorf("agent has finished")
			}
			r.stop = true
			r.cancel()
			return nil
		}
	}
	return fmt.Errorf("agent not found")
}

// RetryWorkflowAgent restarts an unanswered workflow agent call. The script
// keeps waiting for that call; already delivered values cannot be changed.
func (m *Manager) RetryWorkflowAgent(workflow, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		if r.ID == id && r.WorkflowID == workflow {
			if r.Status != "running" || r.child == nil || r.stop {
				return fmt.Errorf("agent is no longer retryable")
			}
			r.retry = true
			r.stop = true
			r.cancel()
			return nil
		}
	}
	return fmt.Errorf("agent not found")
}

func (m *Manager) Observe(ref string) (Record, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		if r.ID == ref {
			return cloneRecord(r.Record), true
		}
	}
	return Record{}, false
}

func (m *Manager) StopObserved(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		if r.ID == id {
			if r.Active() {
				r.stop = true
				r.cancel()
			}
			return nil
		}
	}
	return fmt.Errorf("agent not found: %s", id)
}

func (m *Manager) SteerObserved(id, message string) error {
	m.mu.Lock()
	var target *run
	for _, r := range m.runs {
		if r.ID == id {
			target = r
			break
		}
	}
	if target == nil || !target.Active() {
		m.mu.Unlock()
		return fmt.Errorf("agent is not running")
	}
	child := target.child
	if child == nil {
		target.pending = append(target.pending, message)
	}
	m.mu.Unlock()
	if child != nil {
		child.QueueSteer(message, nil)
	}
	m.changed()
	return nil
}
func (m *Manager) loadWorkflow(p WorkflowParams) (WorkflowMeta, string, error) {
	source := p.Script
	path := p.ScriptPath
	if path == "" && source == "" && p.Name != "" {
		if !safeName.MatchString(p.Name) {
			return WorkflowMeta{}, "", fmt.Errorf("unsafe saved workflow name")
		}
		for _, root := range []string{filepath.Join(m.host.CWD, ".pi", "workflows"), filepath.Join(m.host.CWD, ".agents", "workflows"), filepath.Join(m.host.AgentDir, "workflows")} {
			candidate := filepath.Join(root, p.Name+".js")
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
		}
		if path == "" {
			var roots []PackageRoot
			if m.host.InstalledPackages != nil {
				roots = m.host.InstalledPackages()
			}
			files, _ := packageResources(roots, "workflows", m.Settings().PackageWorkflows)
			for _, file := range files {
				if strings.TrimSuffix(filepath.Base(file.path), ".js") == p.Name {
					path = file.path
					break
				}
			}
			if path == "" {
				return WorkflowMeta{}, "", fmt.Errorf("saved workflow not found: %s", p.Name)
			}
		}
	}
	if path != "" {
		if !filepath.IsAbs(path) {
			path = filepath.Join(m.host.CWD, path)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return WorkflowMeta{}, "", err
		}
		source = string(b)
	}
	if len(source) > 524288 {
		return WorkflowMeta{}, "", fmt.Errorf("workflow script exceeds 512 KiB")
	}
	meta, body, err := ExtractMeta(source)
	return meta, body, err
}
func (m *Manager) StartWorkflow(ctx context.Context, p WorkflowParams) (Workflow, error) {
	if p.ResumeFrom != "" && !workflowIDPattern.MatchString(p.ResumeFrom) {
		return Workflow{}, fmt.Errorf("invalid resumeFromRunId")
	}
	meta, body, err := m.loadWorkflow(p)
	if err != nil {
		return Workflow{}, err
	}
	dir, err := m.taskDir()
	if err != nil {
		return Workflow{}, err
	}
	m.mu.Lock()
	if m.closed || !m.settings.Enabled || !m.settings.WorkflowsEnabled {
		m.mu.Unlock()
		return Workflow{}, fmt.Errorf("workflows are disabled")
	}
	wfCtx, cancel := context.WithCancel(m.ctx)
	w := &Workflow{ID: newID("wf_"), Meta: meta, Status: "running", StartedAt: time.Now().UnixMilli(), Args: slices.Clone(p.Args), done: make(chan struct{}), cancel: cancel, labels: map[string]string{}, sem: make(chan struct{}, max(1, min(16, runtime.NumCPU()-2))), journalWriteMu: &sync.Mutex{}}
	if p.ResumeFrom != "" {
		for _, old := range m.workflows {
			if old.ID == p.ResumeFrom {
				if old.Active() {
					m.mu.Unlock()
					cancel()
					return Workflow{}, fmt.Errorf("workflow is still active; stop it before resuming")
				}
				w.replay = slices.Clone(old.Journal)
				break
			}
		}
		if w.replay == nil {
			var e error
			w.replay, e = readWorkflowJournal(filepath.Join(dir, p.ResumeFrom+".workflow.jsonl"))
			if e != nil || len(w.replay) == 0 {
				m.mu.Unlock()
				cancel()
				return Workflow{}, fmt.Errorf("resume run not found: %s", p.ResumeFrom)
			}
		}
		if slices.ContainsFunc(w.replay, func(e JournalEntry) bool { return e.Resumed }) {
			w.replay = nil
			w.replayLive = true
		}
	}
	w.ScriptPath = filepath.Join(dir, w.ID+".js")
	metaParser := literalParser{s: body}
	metaParser.skip()
	source := body[:metaParser.i] + "export " + body[metaParser.i:]
	if err = os.WriteFile(w.ScriptPath, []byte(source), 0600); err != nil {
		m.mu.Unlock()
		cancel()
		return Workflow{}, err
	}
	m.workflows = append(m.workflows, w)
	out := copyWorkflow(w)
	m.wg.Add(1)
	m.mu.Unlock()
	go m.runWorkflow(wfCtx, w, body)
	m.changed()
	return out, nil
}

func (m *Manager) runWorkflow(ctx context.Context, w *Workflow, body string) {
	defer m.wg.Done()
	result, err := m.evalWorkflow(ctx, w, body, w.Args, 0)
	// The workflow owns every in-flight child, even if its script threw early.
	m.mu.Lock()
	var waits []<-chan struct{}
	for _, r := range m.runs {
		if r.WorkflowID == w.ID && r.Active() {
			r.stop = true
			r.cancel()
			waits = append(waits, r.done)
		}
	}
	m.mu.Unlock()
	for _, done := range waits {
		<-done
	}
	m.mu.Lock()
	w.Status = "completed"
	if err != nil {
		w.Status = "failed"
		w.Error = err.Error()
	}
	if ctx.Err() != nil {
		w.Status = "killed"
	}
	w.Result = result
	w.CompletedAt = time.Now().UnixMilli()
	snapshot := copyWorkflow(w)
	if !m.closed && !w.consumed {
		m.notifications = append(m.notifications, completion{w.ID, w.CompletedAt})
		if m.notificationTimer == nil && (m.host.ParentRunning == nil || !m.host.ParentRunning()) {
			m.notificationTimer = time.AfterFunc(40*time.Millisecond, m.flushNotifications)
		}
	}
	w.cancel()
	close(w.done)
	m.mu.Unlock()
	if dir, e := m.taskDir(); e == nil {
		if b, e := json.Marshal(snapshot); e == nil {
			if e = os.WriteFile(filepath.Join(dir, w.ID+".json"), b, 0600); e != nil {
				m.mu.Lock()
				m.warnings = append(m.warnings, e.Error())
				m.mu.Unlock()
			}
		}
	}
	m.changed()
}

func (m *Manager) evalWorkflow(ctx context.Context, w *Workflow, body string, args json.RawMessage, depth int) (json.RawMessage, error) {
	tools := []codemode.Tool{
		{Name: "progress", Execute: func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
			m.mu.Lock()
			w.Progress = append(w.Progress, slices.Clone(raw))
			m.mu.Unlock()
			m.changed()
			return json.RawMessage("null"), nil
		}},
		{Name: "agent", AwaitsInitiation: true, Execute: func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
			return m.workflowAgent(ctx, w, raw)
		}},
		{Name: "workflow", Execute: func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
			if depth >= 1 {
				return nil, fmt.Errorf("workflow() cannot be nested more than one level deep")
			}
			var p WorkflowParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			m.mu.Lock()
			w.nested++
			n := w.nested
			m.mu.Unlock()
			if n > 256 {
				return nil, fmt.Errorf("workflow exceeded its cap of 256 nested calls")
			}
			meta, child, err := m.loadWorkflow(p)
			if err != nil {
				return nil, err
			}
			m.mu.Lock()
			progress, _ := json.Marshal(map[string]any{"type": "workflow_phase", "title": "▸ " + meta.Name})
			w.Progress = append(w.Progress, progress)
			m.mu.Unlock()
			result, err := m.evalWorkflow(ctx, w, child, p.Args, depth+1)
			if err != nil {
				return nil, err
			}
			m.mu.Lock()
			spent := w.OutputTokens
			m.mu.Unlock()
			return json.Marshal(map[string]any{"value": json.RawMessage(result), "spent": spent})
		}},
	}
	sandbox, err := codemode.NewSandbox(codemode.SandboxOptions{Tools: tools, TimeoutMs: math.Inf(1), MemoryLimitBytes: 256 * 1024 * 1024, CacheDir: filepath.Join(m.host.AgentDir, "cache", "subagent-workflows")})
	if err != nil {
		return nil, err
	}
	defer sandbox.Close()
	argsText := "undefined"
	if len(args) > 0 {
		argsText = string(args)
	}
	script := workflowPrelude + "\nconst args=" + argsText + ";\nconst __result=await (async()=>{\n" + body + "\n})();\n__boundary(__result);return __result;"
	result, err := sandbox.Execute(ctx, script, codemode.ExecuteOptions{})
	if err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("workflow: %s", result.Error.Message)
	}
	if len(result.Value) == 0 {
		return json.RawMessage("null"), nil
	}
	return result.Value, nil
}

func (m *Manager) workflowAgent(ctx context.Context, w *Workflow, raw json.RawMessage) (json.RawMessage, error) {
	var p struct {
		Prompt, Label, Phase, Model, AgentType, Isolation, Gate, Resume, Effort string
		Schema                                                                  json.RawMessage
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	m.mu.Lock()
	index := w.journalIndex
	w.journalIndex++
	nullable := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	var schema any
	if len(p.Schema) > 0 {
		schema = string(p.Schema)
	}
	canonical, _ := json.Marshal([]any{p.Prompt, nullable(p.Label), nullable(p.Model), nullable(p.AgentType), nullable(p.Effort), nullable(p.Isolation), nullable(p.Gate), nullable(p.Resume), schema})
	key := fmt.Sprintf("%x", sha256.Sum256(canonical))
	if index >= 1000 {
		m.mu.Unlock()
		codemode.CallInitiated(ctx)
		return nil, fmt.Errorf("workflow exceeded its cap of 1000 agents")
	}
	if !w.replayLive && index < len(w.replay) && w.replay[index].Key == key && w.replay[index].OK {
		entry := w.replay[index]
		w.Journal = append(w.Journal, entry)
		spent := w.OutputTokens
		m.mu.Unlock()
		codemode.CallInitiated(ctx)
		m.writeWorkflowJournal(w, entry)
		return json.Marshal(map[string]any{"value": entry.Value, "spent": spent})
	}
	w.replayLive = true
	w.Journal = append(w.Journal, JournalEntry{Index: index, Key: key, Resumed: p.Resume != ""})
	w.TotalCount++
	resume := ""
	if p.Resume != "" {
		resume = w.labels[p.Resume]
	}
	m.mu.Unlock()
	codemode.CallInitiated(ctx)
	select {
	case w.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-w.sem }()
	for {
		m.mu.Lock()
		paused := w.pause
		m.mu.Unlock()
		if paused == nil {
			break
		}
		select {
		case <-paused:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if p.Resume != "" && resume == "" {
		return nil, fmt.Errorf("resume label not found: %s", p.Resume)
	}
	params := Params{Prompt: p.Prompt, Description: p.Label, Type: p.AgentType, Model: p.Model, Thinking: p.Effort, Isolation: p.Isolation, Schema: p.Schema, Background: new(false), Resume: resume, Gate: p.Gate}
	var r Record
	var err error
	var tokens, outputTokens int64
	for {
		r, err = m.Spawn(ctx, params, "", w.ID)
		retry := false
		if err == nil {
			m.mu.Lock()
			if p.Label != "" {
				w.labels[p.Label] = r.ID
			}
			var live *run
			for _, candidate := range m.runs {
				if candidate.ID == r.ID {
					live = candidate
					break
				}
			}
			done := live.done
			m.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-done:
			}
			m.mu.Lock()
			r = cloneRecord(live.Record)
			retry = live.retry
			m.mu.Unlock()
			tokens += r.Tokens
			outputTokens += r.OutputTokens
		}
		if !retry || ctx.Err() != nil {
			break
		}
		// A retry starts fresh with the same task, schema and gate, while preserving
		// the workflow's call index and the token cost of the abandoned attempt.
		for {
			m.mu.Lock()
			paused := w.pause
			m.mu.Unlock()
			if paused == nil {
				break
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-paused:
			}
		}
	}
	var result json.RawMessage = json.RawMessage("null")
	ok := err == nil && r.Status == "completed"
	if ok {
		if len(p.Schema) > 0 {
			result = r.StructuredJSON
			if len(result) == 0 {
				ok = false
				result = json.RawMessage("null")
			}
		} else {
			result, _ = json.Marshal(r.Result)
		}
	}
	m.mu.Lock()
	w.DoneCount++
	w.Tokens += tokens
	w.OutputTokens += outputTokens
	entry := JournalEntry{Index: index, Key: key, Value: result, OK: ok, Resumed: p.Resume != ""}
	w.Journal[index] = entry
	spent := w.OutputTokens
	progress, _ := json.Marshal(map[string]any{"type": "workflow_agent", "id": r.ID, "label": p.Label, "phaseTitle": p.Phase, "status": r.Status, "error": r.Error})
	w.Progress = append(w.Progress, progress)
	m.mu.Unlock()
	m.writeWorkflowJournal(w, entry)
	m.changed()
	return json.Marshal(map[string]any{"value": result, "spent": spent})
}

func (m *Manager) writeWorkflowJournal(w *Workflow, entry JournalEntry) {
	if dir, e := m.taskDir(); e == nil {
		w.journalWriteMu.Lock()
		file, e := os.OpenFile(filepath.Join(dir, w.ID+".workflow.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e == nil {
			encoded, _ := json.Marshal(entry)
			_, e = file.Write(append(encoded, '\n'))
			closeErr := file.Close()
			if e == nil {
				e = closeErr
			}
		}
		w.journalWriteMu.Unlock()
		if e != nil {
			m.mu.Lock()
			m.warnings = append(m.warnings, "Workflow journal: "+e.Error())
			m.mu.Unlock()
		}
	}
}

func readWorkflowJournal(path string) ([]JournalEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scan := bufio.NewScanner(file)
	scan.Buffer(make([]byte, 4096), 16*1024*1024)
	entries := map[int]JournalEntry{}
	highest := -1
	for scan.Scan() {
		var entry JournalEntry
		if json.Unmarshal(scan.Bytes(), &entry) != nil {
			continue
		}
		if entry.Index < 0 || entry.Index >= 1000 {
			continue
		}
		entries[entry.Index] = entry
		highest = max(highest, entry.Index)
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	out := make([]JournalEntry, highest+1)
	for i, e := range entries {
		out[i] = e
	}
	return out, nil
}
