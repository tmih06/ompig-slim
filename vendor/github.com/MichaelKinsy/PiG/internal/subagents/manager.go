package subagents

// Ports pi-subagents src/agent-manager.ts, agent-runner.ts, invocation-config.ts,
// completion-outbox.ts, usage.ts and agent-record-entry.ts at the pinned revision.
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofrs/flock"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

type Child interface {
	Send(context.Context, string) ([]agent.AgentMessage, error)
	QueueSteer(string, []ai.ImageContent)
	Messages() []agent.AgentMessage
	Path() string
	Close() error
	Subscribe(func(agent.AgentEvent)) func()
}

type Invocation struct {
	ID, ParentID, WorkflowID, CWD string
	Config                        Config
	Params                        Params
	Depth                         int
	History                       []agent.AgentMessage
}

type Host struct {
	CWD, AgentDir, SessionID, SessionDir string
	Create                               func(context.Context, Invocation) (Child, error)
	History                              func() []agent.AgentMessage
	Notify                               func(string)
	Persist                              func(Record) error
	Restore                              func() []Record
	Changed                              func()
	Event                                func(string, any)
	RefreshTools                         func() error
	ParentRunning                        func() bool
	Subscribe                            func(func(agent.AgentEvent)) func()
	InstalledPackages                    func() []PackageRoot
	Mention                              func(context.Context, string, string) error
}

// Mention starts an off-screen orchestration clone in model mode; direct mode
// starts the requested type immediately. Running handles always steer directly.
func (m *Manager) Mention(handle, message string) error {
	if r, ok := m.Find(handle); ok {
		if r.Active() {
			return m.Steer(r.ID, message, "")
		}
		_, err := m.Spawn(m.ctx, Params{Prompt: message, Description: r.Description, Type: r.Type, Resume: r.ID, Background: new(true), Detached: true}, "", "")
		return err
	}
	var c Config
	found := false
	for _, config := range m.Configs() {
		if strings.EqualFold(config.Name, handle) && (config.Enabled == nil || *config.Enabled) {
			c = config
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown agent @%s", handle)
	}
	if m.Settings().AgentMentions == "model" && m.host.Mention != nil {
		m.mu.Lock()
		if m.closed || !m.settings.Enabled {
			m.mu.Unlock()
			return fmt.Errorf("subagents are disabled")
		}
		m.wg.Add(1)
		m.mu.Unlock()
		go func() {
			defer m.wg.Done()
			if err := m.host.Mention(m.ctx, c.Name, message); err != nil {
				m.mu.Lock()
				m.warnings = append(m.warnings, "Agent mention: "+err.Error())
				m.mu.Unlock()
				m.changed()
			}
		}()
		return nil
	}
	_, err := m.Spawn(m.ctx, Params{Prompt: message, Description: clip(message, 80), Type: c.Name, Background: new(true), Detached: true}, "", "")
	return err
}

type Params struct {
	Prompt         string          `json:"prompt"`
	Description    string          `json:"description"`
	Name           string          `json:"name,omitempty"`
	Type           string          `json:"subagent_type"`
	Model          string          `json:"model,omitempty"`
	Thinking       string          `json:"thinking,omitempty"`
	MaxTurns       *int            `json:"max_turns,omitempty"`
	Background     *bool           `json:"run_in_background,omitempty"`
	Resume         string          `json:"resume,omitempty"`
	Isolated       *bool           `json:"isolated,omitempty"`
	InheritContext *bool           `json:"inherit_context,omitempty"`
	Isolation      string          `json:"isolation,omitempty"`
	Schedule       string          `json:"schedule,omitempty"`
	Schema         json.RawMessage `json:"-"`
	Gate           string          `json:"-"`
	Detached       bool            `json:"-"`
}

type Record struct {
	Requests       int             `json:"requests,omitempty"`
	LastActivity   int64           `json:"lastActivity,omitempty"`
	Killed         bool            `json:"killed,omitempty"`
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	DisplayName    string          `json:"displayName"`
	Color          string          `json:"color,omitempty"`
	Handle         string          `json:"handle,omitempty"`
	Alias          string          `json:"alias,omitempty"`
	Description    string          `json:"description"`
	Status         string          `json:"status"`
	Result         string          `json:"result,omitempty"`
	Error          string          `json:"error,omitempty"`
	StartedAt      int64           `json:"startedAt"`
	CompletedAt    int64           `json:"completedAt,omitempty"`
	ToolUses       int             `json:"toolUses"`
	Turns          int             `json:"turnCount"`
	MaxTurns       int             `json:"maxTurns,omitempty"`
	Tokens         int64           `json:"tokens"`
	OutputTokens   int64           `json:"outputTokens"`
	Cost           float64         `json:"cost"`
	LifetimeUsage  ai.Usage        `json:"lifetimeUsage"`
	ContextTokens  int64           `json:"contextTokens"`
	ContextWindow  int             `json:"contextWindow,omitempty"`
	Compactions    int             `json:"compactionCount"`
	Activity       string          `json:"activity,omitempty"`
	ModelName      string          `json:"modelName,omitempty"`
	Thinking       string          `json:"thinking,omitempty"`
	Background     bool            `json:"isBackground"`
	ParentID       string          `json:"parentAgentId,omitempty"`
	WorkflowID     string          `json:"workflowId,omitempty"`
	Depth          int             `json:"depth"`
	SessionFile    string          `json:"sessionFile,omitempty"`
	OutputFile     string          `json:"outputFile,omitempty"`
	WorktreePath   string          `json:"worktreePath,omitempty"`
	WorktreeBranch string          `json:"worktreeBranch,omitempty"`
	WorktreeLost   bool            `json:"worktreeLost,omitempty"`
	Config         Config          `json:"config"`
	Params         Params          `json:"params"`
	StructuredJSON json.RawMessage `json:"structuredJson,omitempty"`
}

func (r Record) Active() bool   { return r.Status == "running" || r.Status == "queued" }
func (r Record) TopLevel() bool { return r.ParentID == "" && r.WorkflowID == "" }

type run struct {
	reviving bool
	Record
	child         Child
	cancel        context.CancelFunc
	done          chan struct{}
	start         chan struct{}
	handoff       chan struct{}
	handed        bool
	consumed      bool
	pending       []string
	history       []agent.AgentMessage
	pool          string
	retry         bool
	stop          bool
	limited       bool
	activeTools   map[string]string
	transcript    []json.RawMessage
	partial       json.RawMessage
	worktree      *worktree
	evictionTimer *time.Timer
	group         int
	reported      ai.Usage
}

type Manager struct {
	persistMu         sync.Mutex
	hubActivity       []HubActivity
	hubNext           int
	hubSequence       uint64
	hubTools          map[string]int
	mu                sync.Mutex
	mutationMu        sync.Mutex
	host              Host
	settings          Settings
	configs           []Config
	warnings          []string
	runs              []*run
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	closed            bool
	notifications     []completion
	notificationTimer *time.Timer
	bg, fg            int
	jobs              []*Job
	workflows         []*Workflow
	onChange          func()
	unsubscribe       func()
	scheduleLock      *flock.Flock
	groupIndex        int
	pendingUsage      ai.Usage
}

type completion struct {
	id string
	at int64
}

func New(host Host) *Manager {
	s, w := LoadSettings(host.CWD, host.AgentDir)
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{host: host, settings: s, warnings: w, ctx: ctx, cancel: cancel}
	configs, cw := m.reloadConfigs(s)
	m.configs = configs
	m.warnings = append(m.warnings, cw...)
	if host.Restore != nil && s.RememberAgents {
		for _, r := range host.Restore() {
			if r.Active() {
				r.Status = "aborted"
				r.Error = "Session ended before the agent completed"
			}
			done := make(chan struct{})
			close(done)
			m.runs = append(m.runs, &run{Record: r, done: done, handoff: make(chan struct{}), consumed: true})
		}
	}
	m.restoreJobs()
	if host.Subscribe != nil {
		m.unsubscribe = host.Subscribe(func(event agent.AgentEvent) {
			switch e := event.(type) {
			case agent.TurnStartEvent:
				m.mu.Lock()
				m.groupIndex++
				m.mu.Unlock()
			case agent.TurnEndEvent:
				if e.Message.Assistant == nil || e.Message.Assistant.StopReason != "error" && e.Message.Assistant.StopReason != "aborted" {
					m.flushNotifications()
				}
			case agent.AgentEndEvent, agent.AgentSettledEvent:
				m.mu.Lock()
				if len(m.notifications) > 0 && m.notificationTimer == nil && !m.closed {
					m.notificationTimer = time.AfterFunc(40*time.Millisecond, m.flushNotifications)
				}
				m.mu.Unlock()
			}
		})
	}
	return m
}

func newID(prefix string) string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b[:])
}
func (m *Manager) Settings() Settings { m.mu.Lock(); defer m.mu.Unlock(); return m.settings }
func (m *Manager) Configs() []Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.configs)
}
func (m *Manager) Warnings() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.warnings)
}
func (m *Manager) Records() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	records := make([]Record, 0, len(m.runs))
	for _, r := range m.runs {
		records = append(records, cloneRecord(r.Record))
	}
	return records
}
func cloneRecord(r Record) Record { r.StructuredJSON = slices.Clone(r.StructuredJSON); return r }
func (m *Manager) findLocked(ref, owner string) *run {
	ref = strings.TrimPrefix(strings.ToLower(ref), "@")
	for _, r := range m.runs {
		if r.ParentID != owner {
			continue
		}
		if owner == "" && r.WorkflowID != "" {
			continue
		}
		if strings.EqualFold(r.ID, ref) || owner == "" && (strings.EqualFold(r.Handle, ref) || strings.EqualFold(r.Alias, ref) && r.Alias != "") {
			return r
		}
	}
	return nil
}
func (m *Manager) Find(ref string) (Record, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.findLocked(ref, "")
	if r == nil {
		return Record{}, false
	}
	return cloneRecord(r.Record), true
}
func (m *Manager) SetChanged(fn func()) { m.mu.Lock(); m.onChange = fn; m.mu.Unlock() }
func (m *Manager) changed() {
	m.mu.Lock()
	fn := m.onChange
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
	if m.host.Changed != nil {
		m.host.Changed()
	}
}
func (m *Manager) SetInvocation(id, model string, window int, thinking string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		if r.ID == id {
			r.ModelName = model
			r.ContextWindow = window
			r.Thinking = thinking
			return
		}
	}
}
func (m *Manager) SetStructured(id string, raw json.RawMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		if r.ID == id {
			r.StructuredJSON = slices.Clone(raw)
			return
		}
	}
}
func (m *Manager) emit(name string, payload any) {
	if r, ok := payload.(Record); ok {
		if !r.TopLevel() {
			return
		}
		data := map[string]any{"id": r.ID, "type": r.Type, "description": r.Description}
		if name == "subagents:completed" || name == "subagents:failed" {
			data["result"], data["error"], data["status"], data["toolUses"] = r.Result, r.Error, r.Status, r.ToolUses
			data["durationMs"] = r.CompletedAt - r.StartedAt
			if r.Tokens > 0 {
				data["tokens"] = map[string]any{"input": r.LifetimeUsage.Input, "output": r.LifetimeUsage.Output, "total": r.Tokens}
			}
			if r.LifetimeUsage.TotalTokens > 0 || r.LifetimeUsage.Cost.Total > 0 {
				data["usage"] = r.LifetimeUsage
			}
		}
		payload = data
	}
	if m.host.Event != nil {
		m.host.Event(name, payload)
	}
}

// DisableConflictingWorkflows stands down when another orchestrator is installed.
func (m *Manager) DisableConflictingWorkflows() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.WorkflowsEnabled = false
	warning := "Built-in workflows disabled: another extension provides Workflow or SubagentWorkflow. Set workflowsEnabled explicitly to override."
	if !slices.Contains(m.warnings, warning) {
		m.warnings = append(m.warnings, warning)
	}
}

// Update persists before applying; a failed write leaves the prior state intact.
func (m *Manager) Update(changes map[string]any) error {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return errors.New("subagents session is closed")
	}
	if err := SaveSettings(m.host.CWD, changes); err != nil {
		return err
	}
	s, w := LoadSettings(m.host.CWD, m.host.AgentDir)
	configs, cw := m.reloadConfigs(s)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("subagents session is closed")
	}
	m.settings = s
	if !s.ReportUsage {
		m.pendingUsage = ai.Usage{}
	}
	m.configs = configs
	m.warnings = append(w, cw...)
	var waits []<-chan struct{}
	if !s.Enabled {
		for _, r := range m.runs {
			if r.Active() {
				r.consumed = true
				r.stop = true
				r.cancel()
				waits = append(waits, r.done)
			}
		}
		for _, wf := range m.workflows {
			if wf.Active() {
				wf.cancel()
				waits = append(waits, wf.done)
			}
		}
	}
	m.admitLocked()
	m.mu.Unlock()
	for _, done := range waits {
		<-done
	}
	if m.host.RefreshTools != nil {
		if err := m.host.RefreshTools(); err != nil {
			return err
		}
	}
	m.changed()
	m.emit("subagents:settings_changed", map[string]any{"settings": s, "persisted": true})
	return nil
}

func (m *Manager) Close() {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.cancel()
	if m.notificationTimer != nil {
		m.notificationTimer.Stop()
	}
	m.notifications = nil
	for _, r := range m.runs {
		if r.evictionTimer != nil {
			r.evictionTimer.Stop()
		}
	}
	m.mu.Unlock()
	m.wg.Wait()
	if m.unsubscribe != nil {
		m.unsubscribe()
	}
	if m.scheduleLock != nil {
		_ = m.scheduleLock.Unlock()
	}
	m.mu.Lock()
	var children []Child
	for _, r := range m.runs {
		if r.child != nil {
			children = append(children, r.child)
			r.child = nil
		}
	}
	m.mu.Unlock()
	for _, c := range children {
		_ = c.Close()
	}
}

func (m *Manager) Spawn(ctx context.Context, p Params, owner, workflowID string) (Record, error) {
	return m.spawn(ctx, p, owner, workflowID, false)
}

// observedResume permits a human to resume an existing child after its parent
// settles. Model callers always use Spawn and retain delegation checks.
func (m *Manager) spawn(ctx context.Context, p Params, owner, workflowID string, observedResume bool) (Record, error) {
	observedResume = observedResume && p.Resume != ""
	if strings.TrimSpace(p.Prompt) == "" {
		return Record{}, errors.New("prompt must not be empty")
	}
	if p.MaxTurns != nil && (*p.MaxTurns < 1 || *p.MaxTurns > 10000) {
		return Record{}, errors.New("max_turns must be between 1 and 10000")
	}
	if p.Isolation != "" && p.Isolation != "off" && p.Isolation != "worktree" {
		return Record{}, errors.New("isolation must be off or worktree")
	}
	if p.Name != "" && (!safeHandle.MatchString(p.Name) || strings.EqualFold(p.Name, "main")) {
		return Record{}, errors.New("name must use letters, digits, underscores or hyphens and cannot be main")
	}
	if p.Type == "" {
		p.Type = "general-purpose"
	}
	if p.Schedule != "" {
		return m.schedule(p)
	}
	m.mu.Lock()
	if !m.settings.Enabled || m.closed {
		m.mu.Unlock()
		return Record{}, errors.New("subagents are disabled; enable with /subagents on")
	}
	if m.settings.StrictAgentFiles && len(m.warnings) > 0 {
		m.mu.Unlock()
		return Record{}, fmt.Errorf("invalid agent files: %s", strings.Join(m.Warnings(), "; "))
	}
	var parent *run
	depth := 1
	if owner != "" && !observedResume {
		for _, r := range m.runs {
			if r.ID == owner {
				parent = r
				break
			}
		}
		if parent == nil || !parent.Active() {
			m.mu.Unlock()
			return Record{}, errors.New("parent agent is no longer running")
		}
		depth = parent.Depth + 1
		if depth > m.settings.MaxSubagentDepth {
			m.mu.Unlock()
			return Record{}, errors.New("maximum subagent depth reached")
		}
		allow := parent.Config.AllowedSubagents
		if !slices.Contains(allow, "all") && !slices.ContainsFunc(allow, func(s string) bool { return strings.EqualFold(s, p.Type) }) {
			m.mu.Unlock()
			return Record{}, fmt.Errorf("parent cannot delegate to %q", p.Type)
		}
	}
	c, note, err := resolveConfig(m.configs, p.Type, m.settings.FallbackSubagent)
	if err != nil {
		m.mu.Unlock()
		return Record{}, err
	}
	if owner != "" && !observedResume && !slices.Contains(parent.Config.AllowedSubagents, "all") && !slices.ContainsFunc(parent.Config.AllowedSubagents, func(s string) bool { return strings.EqualFold(s, c.Name) }) {
		m.mu.Unlock()
		return Record{}, fmt.Errorf("fallback agent %q is outside parent's allowlist", c.Name)
	}
	if p.Resume != "" {
		r := m.findLocked(p.Resume, owner)
		if workflowID != "" {
			for _, candidate := range m.runs {
				if candidate.ID == p.Resume && candidate.WorkflowID == workflowID {
					r = candidate
					break
				}
			}
		}
		if r == nil || r.WorkflowID != workflowID {
			m.mu.Unlock()
			return Record{}, fmt.Errorf("agent not found: %s", p.Resume)
		}
		if r.Active() {
			m.mu.Unlock()
			return Record{}, errors.New("agent is still running; use steer_subagent")
		}
		if r.reviving {
			m.mu.Unlock()
			return Record{}, errors.New("agent session is reopening; try again shortly")
		}
		if r.Killed {
			m.mu.Unlock()
			return Record{}, errors.New("agent was killed; start a new agent")
		}
		if r.WorktreeLost {
			m.mu.Unlock()
			return Record{}, errors.New("agent's worktree was removed; start a fresh agent")
		}
		c = r.Config
		p.Type = r.Type
		p.Model = r.Params.Model
		p.Thinking = r.Params.Thinking
		p.Isolation = r.Params.Isolation
		p.Schema = r.Params.Schema
		if p.MaxTurns == nil {
			p.MaxTurns = r.Params.MaxTurns
		}
		p = resolveInvocation(c, p, m.settings, owner != "")
		r.Params = p
		r.MaxTurns = value(p.MaxTurns, m.settings.DefaultMaxTurns)
		r.Background = value(p.Background, false)
		r.Status = "queued"
		r.StartedAt = time.Now().UnixMilli()
		r.CompletedAt = 0
		r.Result = ""
		r.Error = ""
		r.done = make(chan struct{})
		r.start = make(chan struct{})
		r.handoff = make(chan struct{})
		r.handed = false
		r.stop = false
		r.Killed = false
		r.retry = false
		r.consumed = false
		r.limited = false
		if r.Requests == 0 {
			r.Requests = r.Turns
		}
		r.Turns = 0
		r.group = m.groupIndex
		r.StructuredJSON = nil
		r.pool = ""
		runCtx, cancel := context.WithCancel(m.ctx)
		r.cancel = cancel
		m.wg.Add(1)
		go m.execute(runCtx, r)
		m.admitLocked()
		out := cloneRecord(r.Record)
		m.mu.Unlock()
		m.changed()
		return out, nil
	}
	p = resolveInvocation(c, p, m.settings, owner != "")
	if p.Thinking != "" && !slices.Contains([]string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}, p.Thinking) {
		m.mu.Unlock()
		return Record{}, fmt.Errorf("invalid thinking level %q", p.Thinking)
	}
	handle := strings.ToLower(c.Name)
	if !safeHandle.MatchString(handle) {
		handle = "agent"
	}
	base := handle
	used := func(s string) bool {
		return slices.ContainsFunc(m.runs, func(r *run) bool {
			return strings.EqualFold(r.Handle, s) || r.Alias != "" && strings.EqualFold(r.Alias, s)
		})
	}
	for i := 2; used(handle); i++ {
		handle = fmt.Sprintf("%s-%d", base, i)
	}
	if p.Name != "" && (used(p.Name) || strings.EqualFold(p.Name, handle)) {
		m.mu.Unlock()
		return Record{}, fmt.Errorf("agent name %q is already in use", p.Name)
	}
	if p.Description == "" {
		p.Description = clip(p.Prompt, 80)
	}
	r := &run{Record: Record{ID: newID("agent_"), Type: c.Name, DisplayName: c.DisplayName, Color: c.Color, Handle: handle, Alias: p.Name, Description: p.Description, Status: "queued", StartedAt: time.Now().UnixMilli(), MaxTurns: value(p.MaxTurns, m.settings.DefaultMaxTurns), Background: value(p.Background, false), ParentID: owner, WorkflowID: workflowID, Depth: depth, Config: c, Params: p, Result: note}, done: make(chan struct{}), start: make(chan struct{}), handoff: make(chan struct{}), activeTools: map[string]string{}, group: m.groupIndex}
	if value(c.OutputTranscript, m.settings.OutputTranscript) {
		if dir, e := m.taskDir(); e == nil {
			r.OutputFile = filepath.Join(dir, r.ID+".output")
			if f, e := os.OpenFile(r.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); e == nil {
				_ = f.Close()
			}
		}
	}
	if value(p.InheritContext, false) {
		if parent != nil && parent.child != nil {
			r.history = parent.child.Messages()
		} else if m.host.History != nil {
			r.history = m.host.History()
		}
	}
	runCtx, cancel := context.WithCancel(m.ctx)
	r.cancel = cancel
	m.runs = append(m.runs, r)
	m.hubEventLocked(r, "lifecycle", "Created", r.Description, "pending", 0)
	m.wg.Add(1)
	go m.execute(runCtx, r)
	m.admitLocked()
	out := cloneRecord(r.Record)
	m.mu.Unlock()
	m.emit("subagents:created", out)
	m.changed()
	return out, nil
}

func resolveInvocation(c Config, p Params, s Settings, nested bool) Params {
	if c.Model != "" {
		p.Model = c.Model
	}
	if c.Thinking != "" {
		p.Thinking = c.Thinking
	}
	if c.MaxTurns != nil {
		p.MaxTurns = c.MaxTurns
	}
	if c.InheritContext != nil {
		p.InheritContext = c.InheritContext
	}
	if c.RunInBackground != nil {
		p.Background = c.RunInBackground
	}
	if p.Background == nil {
		v := s.BackgroundByDefault && !nested
		p.Background = &v
	}
	if c.Isolated != nil {
		p.Isolated = c.Isolated
	}
	if c.Isolation != "" {
		p.Isolation = c.Isolation
	}
	if !s.WorktreeIsolation {
		p.Isolation = "off"
	}
	return p
}
func value[T any](v *T, fallback T) T {
	if v != nil {
		return *v
	}
	return fallback
}
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// Pools are independent. Nested and workflow children bypass the session pools
// so a parent never holds the last slot while waiting for its own child.
func (m *Manager) admitLocked() {
	if m.closed || !m.settings.Enabled {
		return
	}
	for _, r := range m.runs {
		if r.Status != "queued" || r.stop {
			continue
		}
		pool := "background"
		if !r.Background {
			pool = "foreground"
		}
		if r.ParentID != "" || r.WorkflowID != "" || r.Params.Resume != "" || r.Params.Detached {
			pool = "owned"
		}
		if pool == "background" && m.bg >= m.settings.MaxConcurrent {
			continue
		}
		if pool == "foreground" && m.settings.MaxConcurrentForeground > 0 && m.fg >= m.settings.MaxConcurrentForeground {
			continue
		}
		r.pool = pool
		r.Status = "running"
		close(r.start)
		if pool == "background" {
			m.bg++
		}
		if pool == "foreground" {
			m.fg++
		}
	}
}

func (m *Manager) execute(ctx context.Context, r *run) {
	defer m.wg.Done()
	select {
	case <-ctx.Done():
		m.finish(r, "stopped", "", ctx.Err())
		return
	case <-r.start:
	}
	m.emit("subagents:started", m.snapshot(r))
	m.mu.Lock()
	c, p, history, child := r.Config, r.Params, r.history, r.child
	id, owner, wf, depth := r.ID, r.ParentID, r.WorkflowID, r.Depth
	m.mu.Unlock()
	cwd := m.host.CWD
	if p.Isolation == "worktree" && child == nil {
		wt, err := createWorktree(ctx, cwd, id)
		if err != nil {
			m.finish(r, "error", "", err)
			return
		}
		m.mu.Lock()
		r.worktree = wt
		r.WorktreePath = wt.path
		r.WorktreeBranch = wt.branch
		m.mu.Unlock()
		cwd = wt.workPath
	}
	if child == nil {
		var err error
		child, err = m.host.Create(ctx, Invocation{ID: id, ParentID: owner, WorkflowID: wf, CWD: cwd, Config: c, Params: p, Depth: depth, History: history})
		if err != nil {
			m.finish(r, "error", "", err)
			return
		}
		m.mu.Lock()
		r.child = child
		r.SessionFile = child.Path()
		pending := r.pending
		r.pending = nil
		m.mu.Unlock()
		for _, s := range pending {
			child.QueueSteer(s, nil)
		}
	}
	m.mu.Lock()
	r.transcript = nil
	for _, msg := range child.Messages() {
		b, _ := json.Marshal(msg)
		r.transcript = append(r.transcript, b)
	}
	output := m.settings.OutputTranscript
	if c.OutputTranscript != nil {
		output = *c.OutputTranscript
	}
	if output {
		if dir, err := m.taskDir(); err == nil {
			r.OutputFile = filepath.Join(dir, id+".output")
		}
	}
	m.mu.Unlock()
	unsub := child.Subscribe(func(event agent.AgentEvent) { m.onEvent(r, event) })
	_, err := child.Send(ctx, p.Prompt)
	if len(p.Schema) > 0 {
		m.mu.Lock()
		missing := len(r.StructuredJSON) == 0
		m.mu.Unlock()
		if missing && err == nil && ctx.Err() == nil {
			_, err = child.Send(ctx, "You must return your final result by calling StructuredOutput with a payload matching the schema.")
		}
	}
	if p.Gate != "" && err == nil && ctx.Err() == nil {
		gateCtx, gateCancel := context.WithTimeout(ctx, 10*time.Minute)
		cmd := exec.CommandContext(gateCtx, "sh", "-c", p.Gate)
		cmd.Dir = cwd
		b, gateErr := cmd.CombinedOutput()
		gateCancel()
		if gateErr != nil {
			err = fmt.Errorf("gate failed: %w\n%s", gateErr, clip(string(b), 16000))
		}
	}
	unsub()
	text := ""
	messages := child.Messages()
	for i := len(messages) - 1; i >= 0; i-- {
		if a := messages[i].Assistant; a != nil {
			text = assistantText(a)
			if a.StopReason == "error" && err == nil {
				err = errors.New(a.ErrorMessage)
			}
			break
		}
	}
	m.mu.Lock()
	limited, stopped := r.limited, r.stop
	m.mu.Unlock()
	status := "completed"
	if limited {
		status = "steered"
	}
	if err != nil {
		status = "error"
	}
	if ctx.Err() != nil {
		status = "aborted"
	}
	if stopped {
		status = "stopped"
	}
	// A parent owns every child it created, including children still queued.
	m.finish(r, status, text, err)
}

func assistantText(a *agent.AssistantMessage) string {
	var parts []string
	for _, block := range a.Content {
		if t, ok := block.(ai.TextContent); ok {
			parts = append(parts, t.Text)
		}
	}
	return strings.Join(parts, "\n")
}
func (m *Manager) snapshot(r *run) Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneRecord(r.Record)
}

func (m *Manager) onEvent(r *run, event agent.AgentEvent) {
	m.mu.Lock()
	var steer string
	var abort context.CancelFunc
	var eventName string
	var eventData any
	switch e := event.(type) {
	case agent.TurnStartEvent:
		r.Turns++
		if r.MaxTurns > 0 && r.Turns >= r.MaxTurns && !r.limited {
			r.limited = true
			steer = "You have reached your turn limit. Wrap up immediately — provide your final answer now."
		} else if r.limited && r.Turns >= r.MaxTurns+m.settings.GraceTurns {
			abort = r.cancel
		}
	case agent.ToolExecutionStartEvent:
		r.ToolUses++
		if r.activeTools == nil {
			r.activeTools = map[string]string{}
		}
		r.activeTools[e.ToolCallID] = e.ToolName
		r.Activity = activity(e.ToolName)
	case agent.ToolExecutionEndEvent:
		delete(r.activeTools, e.ToolCallID)
		r.Activity = "thinking…"
		for _, name := range r.activeTools {
			r.Activity = activity(name)
			break
		}
	case agent.MessageUpdateEvent:
		b, _ := json.Marshal(e.Message)
		r.partial = b
		if a := e.Message.Assistant; a != nil {
			r.Activity = clip(assistantText(a), 120)
		}
	case agent.MessageEndEvent:
		r.partial = nil
		b, _ := json.Marshal(e.Message)
		r.transcript = append(r.transcript, b)
		if e.Message.Assistant != nil {
			r.Requests++
		}
		if a := e.Message.Assistant; a != nil && a.Usage != nil {
			u := a.Usage
			r.Tokens += int64(u.Input + u.Output + u.CacheWrite)
			r.OutputTokens += int64(u.Output)
			r.ContextTokens = int64(u.Input + u.CacheRead + u.CacheWrite)
			r.Cost += u.Cost.Total
			addUsage(&r.LifetimeUsage, *u)
			if m.settings.ReportUsage {
				addUsage(&m.pendingUsage, *u)
			}
			eventName = "subagents:usage"
			ledger := map[string]any{"id": r.ID, "type": r.Type, "description": r.Description, "depth": r.Depth, "usage": *u}
			if r.ModelName != "" {
				ledger["model"] = r.ModelName
			}
			if r.Thinking != "" {
				ledger["thinking"] = r.Thinking
			}
			if r.ParentID != "" {
				ledger["parentAgentId"] = r.ParentID
			}
			if r.WorkflowID != "" {
				ledger["workflowId"] = r.WorkflowID
			}
			eventData = ledger
			owner := r.ParentID
			for owner != "" {
				var parent *run
				for _, candidate := range m.runs {
					if candidate.ID == owner {
						parent = candidate
						break
					}
				}
				if parent == nil {
					break
				}
				parent.Tokens += int64(u.Input + u.Output + u.CacheWrite)
				parent.OutputTokens += int64(u.Output)
				parent.Cost += u.Cost.Total
				addUsage(&parent.LifetimeUsage, *u)
				owner = parent.ParentID
			}
		}
		if r.OutputFile != "" {
			entry, _ := json.Marshal(map[string]any{"isSidechain": true, "agentId": r.ID, "type": e.Message.Role(), "message": e.Message, "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "cwd": m.host.CWD})
			if f, err := os.OpenFile(r.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); err == nil {
				_, err = f.Write(append(entry, '\n'))
				closeErr := f.Close()
				if err != nil || closeErr != nil {
					r.Error = "Could not write output transcript"
				}
			}
		}
	case agent.CompactionEndEvent:
		if !e.Aborted && e.ErrorMessage == "" {
			r.Compactions++
			r.ContextTokens = 0
			if r.TopLevel() {
				eventName = "subagents:compacted"
				eventData = map[string]any{"id": r.ID, "type": r.Type, "description": r.Description, "reason": e.Reason, "tokensBefore": e.TokensBefore, "compactionCount": r.Compactions}
			}
		}
	}
	m.recordHubEventLocked(r, event)
	child := r.child
	m.mu.Unlock()
	if eventName != "" {
		m.emit(eventName, eventData)
	}
	if steer != "" {
		child.QueueSteer(steer, nil)
	}
	if abort != nil {
		abort()
	}
	m.changed()
}
func activity(name string) string {
	switch name {
	case "read":
		return "reading…"
	case "bash":
		return "running command…"
	case "edit":
		return "editing…"
	case "write":
		return "writing…"
	case "grep":
		return "searching…"
	case "find":
		return "finding files…"
	case "ls":
		return "listing…"
	}
	return name + "…"
}

func (m *Manager) finish(r *run, status, text string, err error) {
	m.stopChildren(r.ID)
	m.mu.Lock()
	wt := r.worktree
	r.worktree = nil
	m.mu.Unlock()
	if wt != nil {
		branch, cleanupErr := cleanupWorktree(m.host.CWD, wt, r.Description)
		m.mu.Lock()
		r.WorktreeLost = true
		r.WorktreeBranch = branch
		m.mu.Unlock()
		if cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
			status = "error"
		}
		if branch != "" {
			text += "\n\nChanges preserved on branch: " + branch
		}
	}
	m.mu.Lock()
	r.Status = status
	r.Result = text
	r.CompletedAt = time.Now().UnixMilli()
	if err != nil {
		r.Error = err.Error()
	}
	r.Activity = ""
	r.partial = nil
	completedAt := r.CompletedAt
	if r.evictionTimer != nil {
		r.evictionTimer.Stop()
	}
	if !m.closed && !r.Killed {
		r.evictionTimer = time.AfterFunc(10*time.Minute, func() {
			m.mu.Lock()
			if m.closed || r.Active() || r.CompletedAt != completedAt {
				m.mu.Unlock()
				return
			}
			child := r.child
			r.child = nil
			r.transcript = nil
			r.history = nil
			m.wg.Add(1)
			m.mu.Unlock()
			defer m.wg.Done()
			if child != nil {
				_ = child.Close()
			}
		})
	}
	if r.pool == "background" {
		m.bg--
	}
	if r.pool == "foreground" {
		m.fg--
	}
	r.pool = ""
	r.cancel()
	snapshot := cloneRecord(r.Record)
	if r.Background && r.TopLevel() && !r.consumed && !m.closed {
		m.notifications = append(m.notifications, completion{r.ID, r.CompletedAt})
		if m.notificationTimer == nil && (m.host.ParentRunning == nil || !m.host.ParentRunning()) {
			m.notificationTimer = time.AfterFunc(40*time.Millisecond, m.flushNotifications)
		}
	}
	m.admitLocked()
	close(r.done)
	var release Child
	if r.Killed {
		release = r.child
		r.child = nil
	}
	m.hubEventLocked(r, "lifecycle", status, text, status, max(0, len(r.transcript)-1))
	m.mu.Unlock()
	if release != nil {
		_ = release.Close()
	}
	if m.host.Persist != nil {
		if err := m.persistRecord(snapshot); err != nil {
			m.mu.Lock()
			m.warnings = append(m.warnings, "Persist agent record: "+err.Error())
			m.mu.Unlock()
		}
	}
	m.emit("subagents:"+map[bool]string{true: "completed", false: "failed"}[status == "completed" || status == "steered"], snapshot)
	m.changed()
}
func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;").Replace(s)
}
func (m *Manager) flushNotifications() {
	m.mu.Lock()
	var messages []string
	var retained []completion
	for _, pending := range m.notifications {
		if strings.HasPrefix(pending.id, "wf_") {
			for _, w := range m.workflows {
				if w.ID == pending.id && !w.consumed && !w.Active() && w.CompletedAt == pending.at {
					messages = append(messages, "<task-notification>\n"+xmlEscape(w.ResultText())+"\n</task-notification>")
					w.consumed = true
				}
			}
			continue
		}
		r := m.findLocked(pending.id, "")
		if r != nil && !r.consumed && !r.Active() && r.CompletedAt == pending.at {
			if m.settings.DefaultJoinMode == "group" && time.Now().UnixMilli()-pending.at < 30000 && slices.ContainsFunc(m.runs, func(other *run) bool {
				return other.TopLevel() && other.Background && other.group == r.group && other.Active()
			}) {
				retained = append(retained, pending)
				continue
			}
			messages = append(messages, fmt.Sprintf("<task-notification>\n<task-id>%s</task-id>\n<status>%s</status>\n<summary>%s</summary>\n<result>%s</result>\nUse get_subagent_result for the full result.\n</task-notification>", r.ID, r.Status, xmlEscape(r.Description), xmlEscape(clip(r.Result+r.Error, 1500))))
			r.consumed = true
		}
	}
	m.notifications = retained
	if m.notificationTimer != nil {
		m.notificationTimer.Stop()
	}
	m.notificationTimer = nil
	if len(retained) > 0 && !m.closed {
		m.notificationTimer = time.AfterFunc(time.Second, m.flushNotifications)
	}
	closed := m.closed
	m.mu.Unlock()
	if !closed && len(messages) > 0 && m.host.Notify != nil {
		m.host.Notify(strings.Join(messages, "\n\n"))
	}
}

func addUsage(dst *ai.Usage, u ai.Usage) {
	dst.Input += u.Input
	dst.Output += u.Output
	dst.CacheRead += u.CacheRead
	dst.CacheWrite += u.CacheWrite
	dst.TotalTokens += u.Input + u.Output + u.CacheRead + u.CacheWrite
	dst.Cost.Input += u.Cost.Input
	dst.Cost.Output += u.Cost.Output
	dst.Cost.CacheRead += u.Cost.CacheRead
	dst.Cost.CacheWrite += u.Cost.CacheWrite
	dst.Cost.Total += u.Cost.Total
}

// DrainUsage reports every assistant message once, including work that finished
// between parent tool calls. Scoped child tools never drain the parent's pool.
func (m *Manager) DrainUsage(owner string) *ai.Usage {
	m.mu.Lock()
	defer m.mu.Unlock()
	if owner != "" || !m.settings.ReportUsage || (m.pendingUsage.TotalTokens == 0 && m.pendingUsage.Cost.Total == 0) {
		return nil
	}
	usage := m.pendingUsage
	m.pendingUsage = ai.Usage{}
	return &usage
}

func (m *Manager) ClaimUsage(id, owner string) *ai.Usage {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.settings.ReportUsage {
		return nil
	}
	r := m.findLocked(id, owner)
	if r == nil {
		return nil
	}
	u := r.LifetimeUsage
	p := r.reported
	r.reported = u
	u.Input -= p.Input
	u.Output -= p.Output
	u.CacheRead -= p.CacheRead
	u.CacheWrite -= p.CacheWrite
	u.TotalTokens -= p.TotalTokens
	u.Cost.Input -= p.Cost.Input
	u.Cost.Output -= p.Cost.Output
	u.Cost.CacheRead -= p.Cost.CacheRead
	u.Cost.CacheWrite -= p.Cost.CacheWrite
	u.Cost.Total -= p.Cost.Total
	return &u
}

func (m *Manager) Wait(ctx context.Context, ref, owner string, allowHandoff bool) (Record, error) {
	m.mu.Lock()
	r := m.findLocked(ref, owner)
	if r == nil {
		m.mu.Unlock()
		return Record{}, fmt.Errorf("agent not found: %s", ref)
	}
	done, handoff := r.done, r.handoff
	m.mu.Unlock()
	if !allowHandoff {
		handoff = nil
	}
	select {
	case <-ctx.Done():
		if allowHandoff {
			m.mu.Lock()
			handed := r.handed
			record := cloneRecord(r.Record)
			m.mu.Unlock()
			if handed {
				return record, nil
			}
			m.Stop(ref, owner)
		}
		return Record{}, ctx.Err()
	case <-done:
	case <-handoff:
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !r.Active() {
		r.consumed = true
	}
	return cloneRecord(r.Record), nil
}
func (m *Manager) Consume(ref, owner string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.findLocked(ref, owner)
	if r == nil {
		return Record{}, fmt.Errorf("agent not found: %s", ref)
	}
	if !r.Active() {
		r.consumed = true
	}
	return cloneRecord(r.Record), nil
}
func (m *Manager) Steer(ref, message, owner string) error {
	if strings.TrimSpace(message) == "" {
		return errors.New("steering message must not be empty")
	}
	m.mu.Lock()
	r := m.findLocked(ref, owner)
	if r == nil {
		m.mu.Unlock()
		return fmt.Errorf("agent not found: %s", ref)
	}
	if !r.Active() {
		m.mu.Unlock()
		return fmt.Errorf("agent %s is not running (status: %s)", ref, r.Status)
	}
	child := r.child
	if child == nil {
		r.pending = append(r.pending, message)
	}
	id := r.ID
	m.mu.Unlock()
	if child != nil {
		child.QueueSteer(message, nil)
	}
	m.emit("subagents:steered", map[string]string{"id": id, "message": message})
	m.changed()
	return nil
}
func (m *Manager) Stop(ref, owner string) error {
	m.mu.Lock()
	r := m.findLocked(ref, owner)
	if r == nil {
		m.mu.Unlock()
		return fmt.Errorf("agent not found: %s", ref)
	}
	if r.Active() {
		r.stop = true
		r.cancel()
	}
	m.mu.Unlock()
	m.changed()
	return nil
}
func (m *Manager) stopChildren(owner string) {
	m.mu.Lock()
	var waits []<-chan struct{}
	for _, r := range m.runs {
		if r.ParentID == owner && r.Active() {
			r.stop = true
			r.cancel()
			waits = append(waits, r.done)
		}
	}
	m.mu.Unlock()
	for _, done := range waits {
		<-done
	}
}
func (m *Manager) Background(ref string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	for _, r := range m.runs {
		if ref != "" && r.ID != ref {
			continue
		}
		if r.Status != "running" || r.Background || r.ParentID != "" || r.WorkflowID != "" || r.Params.Resume != "" || r.handed {
			continue
		}
		r.Background = true
		r.handed = true
		if r.pool == "foreground" {
			m.fg--
			m.bg++
			r.pool = "background"
		}
		close(r.handoff)
		changed = true
	}
	m.admitLocked()
	return changed
}
func (m *Manager) Transcript(ref string) ([]json.RawMessage, error) {
	m.mu.Lock()
	r := m.findLocked(ref, "")
	if r == nil {
		for _, candidate := range m.runs {
			if candidate.ID == ref {
				r = candidate
				break
			}
		}
	}
	if r == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("agent not found: %s", ref)
	}
	out := make([]json.RawMessage, len(r.transcript))
	for i, b := range r.transcript {
		out[i] = slices.Clone(b)
	}
	if r.partial != nil {
		out = append(out, slices.Clone(r.partial))
	}
	path := r.SessionFile
	m.mu.Unlock()
	if len(out) == 0 && path != "" {
		return readTranscript(path)
	}
	return out, nil
}
func (m *Manager) taskDir() (string, error) {
	encoded := strings.TrimLeft(strings.NewReplacer("/", "-", "\\", "-", ":", "").Replace(m.host.CWD), "-")
	dir := filepath.Join(os.TempDir(), "pi-subagents-"+fmt.Sprint(os.Getuid()), encoded, m.host.SessionID, "tasks")
	return dir, os.MkdirAll(dir, 0700)
}
