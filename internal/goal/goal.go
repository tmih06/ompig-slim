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

const (
	// BlockerThreshold is how many consecutive reports of the same blocker pause
	// an active goal. Three separates a repeated blocker from a coincidence while
	// bounding the automatic turns wasted on an answer that cannot change.
	BlockerThreshold = 3

	// maxBlockerRunes bounds the stored explanation, so a pasted log cannot flood
	// goal status or the pause notice.
	maxBlockerRunes = 240
)

// State describes the current objective, its optional token budget, and the
// blocker it last reported.
type State struct {
	ID              string `json:"id"`
	Objective       string `json:"objective"`
	Status          Status `json:"status"`
	TokenBudget     int    `json:"tokenBudget,omitempty"`
	TokensUsed      int    `json:"tokensUsed"`
	TimeUsedSeconds int64  `json:"timeUsedSeconds"`
	Pokes           int    `json:"pokes"`
	// Blocker is the last reported blocker as normalized for display, kept after
	// resume so the status still explains why the goal paused.
	Blocker string `json:"blocker,omitempty"`
	// BlockerCount is the current consecutive streak of that blocker. Resume and
	// progress clear it; a different blocker restarts it at one.
	BlockerCount int   `json:"blockerCount"`
	CreatedAt    int64 `json:"createdAt"`
	UpdatedAt    int64 `json:"updatedAt"`
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

// Resume reactivates a paused goal. The blocker text stays as the explanation of
// the pause, but the streak restarts: a human deciding to continue is a new
// attempt, not the fourth report of the same blocker.
func (t *Tracker) Resume() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == nil {
		return errors.New("there is no active goal")
	}
	if t.state.Status != StatusPaused {
		return fmt.Errorf("cannot transition goal from %s to %s", t.state.Status, StatusActive)
	}
	t.state.Status, t.state.BlockerCount = StatusActive, 0
	t.state.UpdatedAt = time.Now().UnixMilli()
	return nil
}

// Blocked records a report that the goal cannot proceed and returns the state
// after it. The threshold pause lives here rather than in a caller, so no surface
// can record a blocker without applying the policy. Reports are accepted only
// for an active goal: a late report cannot restart work a human stopped.
func (t *Tracker) Blocked(reason string) (*State, error) {
	reason = normalizeBlocker(reason)
	if reason == "" {
		return nil, errors.New("a blocker report must say what the goal is missing")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case t.state == nil:
		return nil, errors.New("there is no active goal")
	case t.state.Status != StatusActive:
		return nil, fmt.Errorf("cannot record a blocker for a %s goal", t.state.Status)
	}
	if t.state.BlockerCount > 0 && blockerIdentity(t.state.Blocker) == blockerIdentity(reason) {
		t.state.BlockerCount++
	} else {
		t.state.Blocker, t.state.BlockerCount = reason, 1
	}
	if t.state.BlockerCount >= BlockerThreshold {
		t.state.Status = StatusPaused
	}
	t.state.UpdatedAt = time.Now().UnixMilli()
	return clone(t.state), nil
}

// Progress records an explicit progress outcome and clears the blocker streak.
// Progress is reported, never inferred: a read-only turn that answered the
// objective has made progress even though it wrote no file.
func (t *Tracker) Progress() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case t.state == nil:
		return errors.New("there is no active goal")
	case t.state.Status != StatusActive:
		return fmt.Errorf("cannot record progress for a %s goal", t.state.Status)
	}
	t.state.Blocker, t.state.BlockerCount = "", 0
	t.state.UpdatedAt = time.Now().UnixMilli()
	return nil
}

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

// normalizeBlocker collapses whitespace and truncates the report so the stored
// explanation stays readable; the limit counts runes, so cut text keeps valid
// UTF-8.
func normalizeBlocker(reason string) string {
	reason = strings.Join(strings.Fields(reason), " ")
	runes := []rune(reason)
	if len(runes) > maxBlockerRunes {
		return string(runes[:maxBlockerRunes]) + "…"
	}
	return reason
}

// blockerIdentity is what "the same blocker" compares: the normalized text,
// case-folded. Nothing else is folded, because merging differently worded
// blockers would pause a goal that has moved on to a new problem.
func blockerIdentity(reason string) string { return strings.ToLower(normalizeBlocker(reason)) }

// clone returns an immutable snapshot copy for callers outside the tracker lock.
func clone(state *State) *State {
	if state == nil {
		return nil
	}
	copy := *state
	return &copy
}
