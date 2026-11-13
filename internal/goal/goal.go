// Package goal owns the lifecycle of one autonomous objective for a session.
// The tracker is deliberately independent of PiG's session and TUI packages so
// the command, tool, and future RPC surfaces share one state machine.
package goal

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Status is the externally visible lifecycle state of a goal.
type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusComplete Status = "complete"
	StatusDropped  Status = "dropped"
)

// State describes the current objective and its optional token budget.
type State struct {
	ID              string `json:"id"`
	Objective       string `json:"objective"`
	Status          Status `json:"status"`
	TokenBudget     int    `json:"tokenBudget,omitempty"`
	TokensUsed      int    `json:"tokensUsed"`
	TimeUsedSeconds int64  `json:"timeUsedSeconds"`
	Pokes           int    `json:"pokes"`
	CreatedAt       int64  `json:"createdAt"`
	UpdatedAt       int64  `json:"updatedAt"`
}

var nextID atomic.Uint64

// Tracker serializes lifecycle mutations and returns copies of state, so a
// caller cannot race a transition by retaining a pointer returned from Get.
type Tracker struct {
	mu    sync.Mutex
	state *State
}

// New creates an empty goal tracker.
func New() *Tracker { return &Tracker{} }

// Get returns a snapshot of the current goal, or nil after Drop.
func (t *Tracker) Get() *State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return clone(t.state)
}

// Create starts a goal with a non-empty objective and optional positive budget.
func (t *Tracker) Create(objective string, tokenBudget int) error {
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return errors.New("goal objective must not be empty")
	}
	if tokenBudget < 0 {
		return errors.New("goal token budget must not be negative")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != nil {
		return errors.New("a goal is already active; drop it before creating another")
	}
	now := time.Now().UnixMilli()
	t.state = &State{ID: fmt.Sprintf("goal-%d", nextID.Add(1)), Objective: objective, Status: StatusActive, TokenBudget: tokenBudget, CreatedAt: now, UpdatedAt: now}
	return nil
}

// Poke records one automatic continuation sent to the agent and returns the
// new total. Only the goal extension calls this for an active goal, so a poke
// is never counted for a user-started turn or a paused goal.
func (t *Tracker) Poke() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == nil || t.state.Status != StatusActive {
		return 0
	}
	t.state.Pokes++
	t.state.UpdatedAt = time.Now().UnixMilli()
	return t.state.Pokes
}

// Pause suspends an active goal without discarding its accounting.
func (t *Tracker) Pause() error { return t.transition(StatusPaused, StatusActive) }

// Resume reactivates a paused goal.
func (t *Tracker) Resume() error { return t.transition(StatusActive, StatusPaused) }

// Complete marks the objective complete and leaves its final state queryable.
func (t *Tracker) Complete() error { return t.transition(StatusComplete, StatusActive, StatusPaused) }

// Drop removes the goal and makes the tracker ready for a new objective.
func (t *Tracker) Drop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == nil {
		return errors.New("there is no active goal")
	}
	t.state = nil
	return nil
}

// transition applies one lifecycle transition when the current status is allowed.
func (t *Tracker) transition(next Status, allowed ...Status) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == nil {
		return errors.New("there is no active goal")
	}
	for _, status := range allowed {
		if t.state.Status == status {
			t.state.Status = next
			t.state.UpdatedAt = time.Now().UnixMilli()
			return nil
		}
	}
	return fmt.Errorf("cannot transition goal from %s to %s", t.state.Status, next)
}

// clone returns an immutable snapshot copy for callers outside the tracker lock.
func clone(state *State) *State {
	if state == nil {
		return nil
	}
	copy := *state
	return &copy
}
