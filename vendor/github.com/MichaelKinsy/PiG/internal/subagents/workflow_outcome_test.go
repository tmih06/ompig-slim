package subagents

import (
	"context"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
)

// A child that wraps up inside its grace turns finished normally, so its value
// must reach the workflow journal. Rejecting it silently turns the answer into
// null, which downstream steps read as a successful empty result.
func TestWorkflowKeepsGracePeriodChildValue(t *testing.T) {
	child := &fakeChild{}
	child.send = func(context.Context, string) (string, error) {
		child.mu.Lock()
		listener := child.listener
		child.mu.Unlock()
		for range 3 {
			listener(agent.TurnStartEvent{})
		}
		return "grace answer", nil
	}
	m := testManager(t, func(context.Context, Invocation) (Child, error) { return child, nil })
	if err := m.Update(map[string]any{"defaultMaxTurns": 1, "graceTurns": 5}); err != nil {
		t.Fatal(err)
	}
	wf, err := m.StartWorkflow(context.Background(), WorkflowParams{Script: `export const meta = {name: 'grace', description: 'Grace period value'}; return await agent('work')`})
	if err != nil {
		t.Fatal(err)
	}
	wf, err = m.WaitWorkflow(context.Background(), wf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wf.Journal) == 0 {
		t.Fatal("workflow produced no journal entry")
	}
	entry := wf.Journal[0]
	if !entry.OK || !strings.Contains(string(entry.Value), "grace answer") {
		t.Fatalf("grace-period child value was discarded: ok=%v value=%s", entry.OK, entry.Value)
	}
}
