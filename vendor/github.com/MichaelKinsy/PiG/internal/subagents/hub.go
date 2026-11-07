package subagents

// Native host data for oh-my-pi packages/tui/src/overlays/agent-hub.ts and
// packages/coding-agent/src/modes/agent-hub-runtime.ts at f2b795ba747b23d42052184d822b41e3d01357cc.
import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

const hubActivityLimit = 2000

type HubAgent struct {
	Record
	State      string
	LastActive int64
	Usage      ai.Usage
	Measured   bool
}

type HubActivity struct {
	ID                           uint64
	AgentID                      string
	Timestamp                    int64
	Kind, Title, Summary, Status string
	MessageIndex                 int
	toolKey                      string
}

// HubSnapshot is session scoped, includes owned children, and reports direct
// usage. A parent's lifetime rollup must not bill its children's rows twice.
func (m *Manager) HubSnapshot() []HubAgent {
	m.mu.Lock()
	defer m.mu.Unlock()
	children := map[string]ai.Usage{}
	for _, r := range m.runs {
		if r.ParentID != "" {
			u := children[r.ParentID]
			addUsage(&u, r.LifetimeUsage)
			children[r.ParentID] = u
		}
	}
	out := make([]HubAgent, 0, len(m.runs))
	for _, r := range m.runs {
		state := "parked"
		switch {
		case r.Killed || r.Status == "error" || r.Status == "aborted" || r.Status == "stopped":
			state = "aborted"
		case r.Active() || r.reviving:
			state = "running"
		case r.child != nil:
			state = "idle"
		}
		usage := r.LifetimeUsage
		child := children[r.ID]
		usage.Input = max(0, usage.Input-child.Input)
		usage.Output = max(0, usage.Output-child.Output)
		usage.CacheRead = max(0, usage.CacheRead-child.CacheRead)
		usage.CacheWrite = max(0, usage.CacheWrite-child.CacheWrite)
		usage.Cost.Total = max(0, usage.Cost.Total-child.Cost.Total)
		usage.TotalTokens = usage.Input + usage.Output + usage.CacheWrite
		last := r.LastActivity
		if last == 0 {
			last = max(r.StartedAt, r.CompletedAt)
		}
		if r.Requests == 0 {
			r.Requests = r.Turns
		}
		out = append(out, HubAgent{Record: cloneRecord(r.Record), State: state, LastActive: last, Usage: usage, Measured: r.Turns > 0 || usage.TotalTokens > 0 || usage.Cost.Total > 0})
	}
	return out
}

// Restore the chronological ring across all agent records, preserving event IDs
// so new activity continues after the saved entries rather than colliding.
func (m *Manager) restoreHubActivity() {
	slices.SortFunc(m.hubActivity, func(a, b HubActivity) int { return cmp.Compare(a.ID, b.ID) })
	if len(m.hubActivity) > hubActivityLimit {
		m.hubActivity = slices.Clone(m.hubActivity[len(m.hubActivity)-hubActivityLimit:])
	}
	for i := range m.hubActivity {
		e := &m.hubActivity[i]
		m.hubSequence = max(m.hubSequence, e.ID)
		if e.Status == "pending" {
			e.Status = "aborted"
		}
	}
}

func (m *Manager) HubActivity() []HubActivity {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := slices.Clone(m.hubActivity)
	if len(out) == hubActivityLimit && m.hubNext > 0 {
		out = append(out[m.hubNext:], out[:m.hubNext]...)
	}
	return out
}

func (m *Manager) hubEventLocked(r *run, kind, title, summary, status string, index int) int {
	m.hubSequence++
	event := HubActivity{ID: m.hubSequence, AgentID: r.ID, Timestamp: time.Now().UnixMilli(), Kind: kind, Title: title, Summary: clip(summary, 1200), Status: status, MessageIndex: index}
	slot := len(m.hubActivity)
	if slot < hubActivityLimit {
		m.hubActivity = append(m.hubActivity, event)
	} else {
		slot = m.hubNext
		old := m.hubActivity[slot]
		if old.toolKey != "" {
			if m.hubTools[old.toolKey] == slot {
				delete(m.hubTools, old.toolKey)
			}
		}
		m.hubActivity[slot] = event
		m.hubNext = (slot + 1) % hubActivityLimit
	}
	return slot
}

func (m *Manager) recordHubEventLocked(r *run, event agent.AgentEvent) {
	r.LastActivity = time.Now().UnixMilli()
	switch e := event.(type) {
	case agent.TurnStartEvent:
		m.hubEventLocked(r, "lifecycle", "Turn started", r.Description, "pending", len(r.transcript))
	case agent.ToolExecutionStartEvent:
		slot := m.hubEventLocked(r, "tool", e.ToolName, string(e.Args), "pending", max(0, len(r.transcript)-1))
		if m.hubTools == nil {
			m.hubTools = map[string]int{}
		}
		key := r.ID + ":" + e.ToolCallID
		m.hubTools[key] = slot
		m.hubActivity[slot].toolKey = key
	case agent.ToolExecutionEndEvent:
		if slot, ok := m.hubTools[r.ID+":"+e.ToolCallID]; ok {
			row := &m.hubActivity[slot]
			row.Status = "completed"
			if e.IsError {
				row.Status = "error"
			}
			var parts []string
			for _, block := range e.Result.Content {
				if text, ok := block.(ai.TextContent); ok {
					parts = append(parts, text.Text)
				}
			}
			if len(parts) > 0 {
				row.Summary = clip(strings.Join(parts, "\n"), 1200)
			}
		}
	case agent.MessageEndEvent:
		if a := e.Message.Assistant; a != nil {
			text := assistantText(a)
			if a.StopReason == "error" {
				m.hubEventLocked(r, "response", "Error", a.ErrorMessage, "error", max(0, len(r.transcript)-1))
			}
			if text != "" {
				m.hubEventLocked(r, "response", "Response", text, "completed", max(0, len(r.transcript)-1))
			}
		}
	case agent.CompactionEndEvent:
		if !e.Aborted && e.ErrorMessage == "" {
			m.hubEventLocked(r, "lifecycle", "Compacted", e.Summary, "completed", max(0, len(r.transcript)-1))
		}
	}
}

// PromptObserved is the human-facing prompt route; model tools retain their
// ownership checks. Sending to a settled agent resumes the same saved session.
func (m *Manager) PromptObserved(id, message string) error {
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("message must not be empty")
	}
	r, ok := m.Observe(id)
	if !ok {
		return fmt.Errorf("agent not found")
	}
	if r.Killed {
		return fmt.Errorf("agent was killed; start a new agent")
	}
	if r.Active() {
		return m.SteerObserved(id, message)
	}
	_, err := m.spawn(m.ctx, Params{Prompt: message, Description: r.Description, Type: r.Type, Resume: r.ID, Background: new(true), Detached: true}, r.ParentID, r.WorkflowID, true)
	return err
}

// ReviveObserved reopens a parked session without spending a model turn.
// Work is owned and drained by the manager; it never blocks the TUI loop.
func (m *Manager) ReviveObserved(id string) error {
	m.mu.Lock()
	var target *run
	for _, r := range m.runs {
		if r.ID == id {
			target = r
			break
		}
	}
	if target == nil || m.closed || !m.settings.Enabled {
		m.mu.Unlock()
		return fmt.Errorf("agent is unavailable")
	}
	if target.Killed || target.Active() || target.reviving || target.child != nil {
		m.mu.Unlock()
		return fmt.Errorf("only parked agents can be revived")
	}
	if target.WorktreeLost {
		m.mu.Unlock()
		return fmt.Errorf("agent's worktree was removed; start a new agent")
	}
	if m.host.Create == nil {
		m.mu.Unlock()
		return fmt.Errorf("session host is unavailable")
	}
	target.reviving = true
	if target.evictionTimer != nil {
		target.evictionTimer.Stop()
	}
	params := target.Params
	params.Resume = id
	invocation := Invocation{ID: id, ParentID: target.ParentID, WorkflowID: target.WorkflowID, CWD: m.host.CWD, Config: target.Config, Params: params, Depth: target.Depth}
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		child, err := m.host.Create(m.ctx, invocation)
		var transcript []json.RawMessage
		if err == nil {
			for _, message := range child.Messages() {
				b, _ := json.Marshal(message)
				transcript = append(transcript, b)
			}
		}
		m.mu.Lock()
		target.reviving = false
		keep := err == nil && !m.closed && !target.Killed && !target.Active()
		if keep {
			target.child = child
			target.SessionFile = child.Path()
			target.transcript = transcript
			target.LastActivity = time.Now().UnixMilli()
			m.hubEventLocked(target, "lifecycle", "Revived", "Session reopened without starting a model turn", "completed", 0)
		}
		if err != nil {
			target.Error = err.Error()
		}
		m.mu.Unlock()
		if child != nil && !keep {
			_ = child.Close()
		}
		m.changed()
	}()
	m.changed()
	return nil
}

// KillObserved cancels an active turn and releases its instance. The tombstone
// survives session restoration; closing a child runs outside the input loop.
func (m *Manager) KillObserved(id string) error {
	m.mu.Lock()
	var target *run
	for _, r := range m.runs {
		if r.ID == id {
			target = r
			break
		}
	}
	if target == nil || m.closed {
		m.mu.Unlock()
		return fmt.Errorf("agent is unavailable")
	}
	target.Killed = true
	target.LastActivity = time.Now().UnixMilli()
	m.hubEventLocked(target, "lifecycle", "Killed", "Agent instance released", "aborted", max(0, len(target.transcript)-1))
	if target.Active() {
		target.stop = true
		target.cancel()
		m.mu.Unlock()
		m.changed()
		return nil
	}
	target.Status = "stopped"
	child := target.child
	target.child = nil
	if target.evictionTimer != nil {
		target.evictionTimer.Stop()
	}
	snapshot := cloneRecord(target.Record)
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		if child != nil {
			_ = child.Close()
		}
		if m.host.Persist != nil {
			_ = m.persistRecord(snapshot)
		}
	}()
	m.changed()
	return nil
}

// Serialize lifecycle records and refresh the snapshot after taking the write
// lock, so an older completion cannot overwrite a newer kill tombstone.
func (m *Manager) persistRecord(snapshot Record) error {
	m.persistMu.Lock()
	defer m.persistMu.Unlock()
	m.mu.Lock()
	for _, r := range m.runs {
		if r.ID == snapshot.ID {
			snapshot = cloneRecord(r.Record)
			break
		}
	}
	// Store this agent's retained events with its existing session record.
	// Iterate the ring chronologically, including updated tool results.
	snapshot.HubActivity = nil
	for i := 0; i < len(m.hubActivity); i++ {
		e := m.hubActivity[(m.hubNext+i)%len(m.hubActivity)]
		if e.AgentID == snapshot.ID {
			snapshot.HubActivity = append(snapshot.HubActivity, e)
		}
	}
	m.mu.Unlock()
	if m.host.Persist == nil {
		return nil
	}
	return m.host.Persist(snapshot)
}
