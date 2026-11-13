package codingagent

import (
	"bytes"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

// TestContextBranchRetainsDecodedHistory guards resumed prompt reads: a full
// active-context read must not re-decode the cold prefix on every later turn.
// Moving the leaf still evicts unrelated history under the existing tail bound.
func TestContextBranchRetainsDecodedHistory(t *testing.T) {
	manager := NewSessionManagerWithDir(t.TempDir(), t.TempDir())
	session, err := manager.Create("", "")
	if err != nil {
		t.Fatal(err)
	}
	var first string
	for i := 0; i < retainTailEntries+50; i++ {
		id, err := session.AppendMessage(agent.AgentMessage{User: &agent.UserMessage{Role: "user", Content: ai.UserText("history"), Timestamp: 1}})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = id
		}
	}
	loaded, err := manager.Load(session.path)
	if err != nil {
		t.Fatal(err)
	}
	before := loaded.EntryReads()
	branch := loaded.GetContextBranch()
	after := loaded.EntryReads()
	if after <= before {
		t.Fatal("fixture did not exercise file-backed entries")
	}
	again := loaded.GetContextBranch()
	if len(branch) != len(again) || loaded.EntryReads() != after {
		t.Fatal("repeated context read reparsed history", after, loaded.EntryReads())
	}
	for i := range branch {
		if !bytes.Equal(branch[i].Raw(), again[i].Raw()) {
			t.Fatal("retaining an entry changed its record", i)
		}
	}
	if err := loaded.SetLeafID(&first); err != nil {
		t.Fatal(err)
	}
	if len(loaded.GetContextBranch()) != 1 {
		t.Fatal("leaf move did not replace the active context")
	}
}
