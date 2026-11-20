package coding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
)

func TestImportMissingCwdRemovesCopiedSession(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	cwd, agentDir, sessionDir := t.TempDir(), t.TempDir(), t.TempDir()
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: agentDir, SettingsManager: NewInMemorySettingsManager(Settings{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer services.Close()

	missingCWD := filepath.Join(t.TempDir(), "deleted-project")
	sourceManager := icodingagent.NewSessionManagerWithDir(missingCWD, t.TempDir())
	source, err := sourceManager.Create("", "")
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := source.Path()
	if _, err := source.AppendMessage(agent.AgentMessage{User: &agent.UserMessage{Role: "user", Content: ai.UserText("fixture"), Timestamp: 1}}); err != nil {
		t.Fatal(err)
	}
	importedPath := filepath.Join(sessionDir, filepath.Base(sourcePath))

	session, err := NewSession(services, SessionOptions{
		SessionDir: sessionDir, NoTools: "all", ResourceLoader: NoResources,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	for attempt := 0; attempt < 2; attempt++ {
		_, err := session.ImportFromJsonl(context.Background(), sourcePath, "")
		var missing *icodingagent.MissingSessionCwdError
		if !errors.As(err, &missing) {
			t.Fatalf("import error = %v, want MissingSessionCwdError", err)
		}
		if _, statErr := os.Stat(importedPath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("failed import left copied session at %s: %v", importedPath, statErr)
		}
	}
}
