package codingagent

import (
	"testing"
	"time"
)

func TestSessionSelectorSearchSelectsBestMatchAfterScrolling(t *testing.T) {
	sessions := []SessionInfo{
		{Path: "/sessions/recent.jsonl", Name: "Recent unrelated", Modified: time.Unix(3, 0)},
		{Path: "/sessions/long.jsonl", Name: "alpha zebra", Modified: time.Unix(2, 0)},
		{Path: "/sessions/exact.jsonl", Name: "alpha", Modified: time.Unix(1, 0)},
	}
	selector := newSessionSelectorFromListers(
		func(SessionListOptions) ([]SessionInfo, error) { return sessions, nil },
		func(SessionListOptions) ([]SessionInfo, error) { return sessions, nil },
		nil, nil, "", nil,
	)
	defer selector.close()

	deadline := time.Now().Add(time.Second)
	for selector.scopeLoad(sessionScopeCurrent) != nil && time.Now().Before(deadline) {
		selector.drainLoadUpdates()
		time.Sleep(time.Millisecond)
	}
	if selector.scopeLoad(sessionScopeCurrent) != nil {
		t.Fatal("current sessions did not load")
	}

	selector.HandleInput("\x1b[B")
	selector.HandleInput("a")
	selector.HandleInput("l")
	selector.HandleInput("p")
	selector.HandleInput("h")
	selector.HandleInput("a")
	selector.HandleInput("\r")

	if got, want := selector.SelectedPath(), "/sessions/exact.jsonl"; got != want {
		t.Fatalf("search selected %q, want best match %q", got, want)
	}
}
