package agent

import (
	"slices"
	"sync"
)

// QueueMode controls how PendingMessageQueue.Drain behaves.
// Mirrors upstream packages/agent/src/agent.ts:55 (type QueueMode).
type QueueMode string

const (
	// QueueModeAll drains all queued messages at once.
	QueueModeAll QueueMode = "all"
	// QueueModeOneAtATime drains one message per call.
	QueueModeOneAtATime QueueMode = "one-at-a-time"
)

// Ports packages/agent/src/agent.ts (PendingMessageQueue).
// PendingMessageQueue is a thread-safe FIFO of agent messages with a
// configurable drain policy. Used for steering and follow-up queues.
//
// Mirrors upstream packages/agent/src/agent.ts:113 (class PendingMessageQueue).
type PendingMessageQueue struct {
	mu      sync.Mutex
	entries []queuedEntry
	mode    QueueMode
}

// queuedEntry is one queue slot: a single message, or a group whose members
// must reach the model in the same turn.
//
// pig additive (D254): upstream's queue holds AgentMessage values only, so a
// caller that needs two messages delivered together (a prompt and the file
// mentions read for it) has no way to say so under one-at-a-time mode.
type queuedEntry struct {
	message AgentMessage
	group   []AgentMessage
}

// messages is the entry's messages in delivery order.
func (e queuedEntry) messages() []AgentMessage {
	if e.group != nil {
		return e.group
	}
	return []AgentMessage{e.message}
}

// userMessage is the entry's first user-role message, for ClearUserMessages.
func (e queuedEntry) userMessage() (AgentMessage, bool) {
	for _, message := range e.messages() {
		if message.User != nil {
			return message, true
		}
	}
	return AgentMessage{}, false
}

// NewPendingMessageQueue creates a queue with the given drain mode.
func NewPendingMessageQueue(mode QueueMode) *PendingMessageQueue {
	return &PendingMessageQueue{mode: mode}
}

// Enqueue appends a message to the queue.
func (q *PendingMessageQueue) Enqueue(msg AgentMessage) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.entries = append(q.entries, queuedEntry{message: msg})
}

// EnqueueGroup appends messages that must reach the model in one turn. A plain
// pair of Enqueue calls is split across two drains under one-at-a-time mode, so
// the model would answer the first before seeing the second; a group occupies
// one slot and yields all its members from that slot's drain.
func (q *PendingMessageQueue) EnqueueGroup(msgs ...AgentMessage) {
	if len(msgs) == 0 {
		return
	}
	if len(msgs) == 1 {
		q.Enqueue(msgs[0])
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.entries = append(q.entries, queuedEntry{group: slices.Clone(msgs)})
}

// HasItems reports whether the queue has any pending messages.
func (q *PendingMessageQueue) HasItems() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries) > 0
}

// Peek returns the messages the next Drain would remove, without removing
// them: every message in "all" mode, the oldest one in "one-at-a-time" mode.
// Mirrors upstream PendingMessageQueue.peek.
func (q *PendingMessageQueue) Peek() []AgentMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.peekLocked()
}

func (q *PendingMessageQueue) peekLocked() []AgentMessage {
	entries, _ := q.peekEntriesLocked()
	var out []AgentMessage
	for _, entry := range entries {
		out = append(out, entry.messages()...)
	}
	return out
}

// peekEntriesLocked returns the slots the next Drain removes, with the number
// of messages they expand to, so Drain can advance by slots rather than by
// expanded messages.
func (q *PendingMessageQueue) peekEntriesLocked() ([]queuedEntry, int) {
	if len(q.entries) == 0 {
		return nil, 0
	}
	if q.mode == QueueModeAll {
		return q.entries, len(q.entries)
	}
	return q.entries[:1], 1
}

// Drain removes and returns pending messages according to the queue's mode.
// "all" drains every message; "one-at-a-time" drains the oldest single message.
// Returns nil when the queue is empty. Mirrors upstream PendingMessageQueue.drain.
func (q *PendingMessageQueue) Drain() []AgentMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries, count := q.peekEntriesLocked()
	q.entries = q.entries[count:]
	var drained []AgentMessage
	for _, entry := range entries {
		drained = append(drained, entry.messages()...)
	}
	return drained
}

// Clear removes and returns all queued messages regardless of mode.
func (q *PendingMessageQueue) Clear() []AgentMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	cleared := q.flattenLocked(q.entries)
	q.entries = nil
	return cleared
}

// flattenLocked expands entries to their messages in delivery order.
func (q *PendingMessageQueue) flattenLocked(entries []queuedEntry) []AgentMessage {
	var out []AgentMessage
	for _, entry := range entries {
		out = append(out, entry.messages()...)
	}
	return out
}

// ClearUserMessages removes and returns the queued user messages and keeps every other queued message in order.
func (q *PendingMessageQueue) ClearUserMessages() []AgentMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	var cleared []AgentMessage
	var kept []queuedEntry
	for _, entry := range q.entries {
		if message, ok := entry.userMessage(); ok {
			cleared = append(cleared, message)
			continue
		}
		kept = append(kept, entry)
	}
	q.entries = kept
	return cleared
}

// SetMode changes the drain mode.
func (q *PendingMessageQueue) SetMode(mode QueueMode) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.mode = mode
}

// Mode returns the current drain mode.
func (q *PendingMessageQueue) Mode() QueueMode {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.mode
}

// Len returns the number of queued messages.
func (q *PendingMessageQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.flattenLocked(q.entries))
}

// Messages returns copies of all queued messages without draining, whatever
// the mode. Used for UI display of pending steering/follow-up messages.
func (q *PendingMessageQueue) Messages() []AgentMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.entries) == 0 {
		return nil
	}
	return q.flattenLocked(q.entries)
}
