package coding

import (
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/subagents"
)

// TestResumeReadsContextOnce exercises construction, not a parser shortcut:
// existence checks, settings restoration and the Agent must share one decoded
// active context rather than read the cold prefix repeatedly before the UI opens.
func TestResumeReadsContextOnce(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(Settings{})})
	if err != nil {
		t.Fatal(err)
	}
	defer services.Close()
	manager := icodingagent.NewSessionManagerWithDir(services.CWD(), t.TempDir())
	original, err := manager.Create("", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := original.AppendCustomEntry("subagents:record", subagents.Record{ID: "saved-agent", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 512; i++ {
		if _, err := original.AppendMessage(agent.AgentMessage{User: &agent.UserMessage{Role: "user", Content: ai.UserText("saved history"), Timestamp: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"manager", "resume", "compacted"} {
		t.Run(mode, func(t *testing.T) {
			loaded, err := manager.Load(original.Path())
			if err != nil {
				t.Fatal(err)
			}
			wantMessages := 512
			if mode == "compacted" {
				if _, err := loaded.AppendCompaction("Saved summary", *original.GetLeafID(), 100000, nil, false, nil); err != nil {
					t.Fatal(err)
				}
				loaded, err = manager.Load(original.Path())
				if err != nil {
					t.Fatal(err)
				}
				wantMessages = 2 // summary and the one kept user message
			}
			before := loaded.EntryReads()
			if before == 0 || before >= 512 {
				t.Fatal("fixture must have both resident and cold entries", before)
			}
			opts := SessionOptions{NoTools: "all", ResourceLoader: NoResources}
			if mode != "resume" {
				opts.SessionManager = loaded
			} else {
				opts.ResumePath = original.Path()
				opts.resumed = loaded
			}
			session, err := NewSession(services, opts)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			wantReads := int64(512) - before
			if mode == "compacted" {
				wantReads = 0 // its one active message is already resident
			}
			if reads := loaded.EntryReads() - before; reads != wantReads {
				t.Fatalf("construction decoded %d cold entries, want %d once", reads, wantReads)
			}
			before = loaded.EntryReads()
			session.Inner().GetContextBranch()
			if loaded.EntryReads() != before {
				t.Fatal("the resumed context was not retained for UI hydration")
			}
			records := session.Subagents().Records()
			if len(records) != 1 || records[0].ID != "saved-agent" {
				t.Fatal("resume lost a pre-compaction subagent record", records)
			}
			messages := session.Agent().Messages()
			if len(messages) != wantMessages {
				t.Fatalf("resumed %d messages, want %d", len(messages), wantMessages)
			}
		})
	}
}
