package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
)

func TestForkMissingSessionCwdPreservesOriginal(t *testing.T) {
	fallbackCWD := t.TempDir()
	sessionDir := t.TempDir()
	missingCWD := filepath.Join(t.TempDir(), "deleted-project")
	manager := codingagent.NewSessionManagerWithDir(missingCWD, sessionDir)
	original, err := manager.Create("", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := original.AppendMessage(agent.AgentMessage{User: &agent.UserMessage{Role: "user", Content: ai.UserText("saved turn"), Timestamp: 1}}); err != nil {
		t.Fatal(err)
	}
	originalPath := original.Path()
	originalBytes, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}

	selection := startupSessionSelection{
		runtimeCWD: fallbackCWD,
		sessionDir: sessionDir,
		missingCWD: &missingSessionCWD{sessionFile: originalPath, storedCWD: missingCWD, fallbackCWD: fallbackCWD},
	}
	if err := selection.forkMissingCWD(*selection.missingCWD); err != nil {
		t.Fatal(err)
	}
	if selection.forkPath == "" || selection.forkPath == originalPath {
		t.Fatalf("fork path = %q, want a new session path", selection.forkPath)
	}
	if selection.runtimeCWD != fallbackCWD || selection.missingCWD != nil {
		t.Fatalf("fork did not continue in fallback cwd: %+v", selection)
	}

	forkBytes, err := os.ReadFile(selection.forkPath)
	if err != nil {
		t.Fatal(err)
	}
	var header struct {
		ID            string `json:"id"`
		CWD           string `json:"cwd"`
		ParentSession string `json:"parentSession"`
	}
	headerEnd := bytes.IndexByte(forkBytes, '\n')
	if headerEnd < 0 {
		t.Fatal("fork has no session header line")
	}
	if err := json.Unmarshal(forkBytes[:headerEnd], &header); err != nil {
		t.Fatal(err)
	}
	if header.ID == "" || header.ID == original.ID() || header.CWD != fallbackCWD || header.ParentSession != originalPath {
		t.Fatalf("fork header = %+v, want a new id, fallback cwd, and original parent", header)
	}
	after, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, originalBytes) {
		t.Fatal("fork changed the missing-cwd source session")
	}
}
