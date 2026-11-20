package codingagent

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
	"github.com/MichaelKinsy/PiG/tui"
)

func TestReadImageWarningStaysOutOfModelHistory(t *testing.T) {
	cwd := t.TempDir()
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aMZkAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "x.png"), png, 0o600); err != nil {
		t.Fatal(err)
	}
	tool := &tools.ReadTool{CWD: cwd, AutoResizeImages: new(false)}
	ctx := agent.WithToolEnvironment(t.Context(), agent.ToolEnvironment{Model: "text-only", SupportsImages: new(false)})
	args := json.RawMessage(`{"path":"x.png"}`)
	result, err := tool.Execute(ctx, "read-image", args, nil)
	if err != nil || result.IsError || len(result.Content) != 2 {
		t.Fatalf("read = %v, %v", result, err)
	}
	if strings.Contains(result.Text(), "does not support images") || strings.Contains(string(result.StructuredContent), "does not support images") {
		t.Fatal("capability warning leaked into the stored tool content")
	}
	for _, input := range [][]string{{"text"}, {"text", "image"}} {
		messages := ai.TransformMessages([]ai.Message{ai.ToolResultMessage{ToolCallID: "read-image", Content: result.Content}}, &ai.Model{Input: input}, nil)
		block := messages[0].(ai.ToolResultMessage).Content[1]
		if len(input) == 1 {
			if text, ok := block.(ai.TextContent); !ok || !strings.Contains(text.Text, "tool image omitted") {
				t.Fatalf("text-only model got %T, want request-time placeholder", block)
			}
		} else if _, ok := block.(ai.ImageContent); !ok {
			t.Fatalf("vision model got %T, want original image", block)
		}
	}
	storedDetails, err := json.Marshal(result.Details)
	if err != nil {
		t.Fatal(err)
	}
	for _, details := range []any{result.Details, json.RawMessage(storedDetails)} {
		result.Details = details
		for _, expanded := range []bool{false, true} {
			component := readRenderResult(result, extension.ToolRenderResultOptions{Expanded: expanded}, tui.ActiveTheme(), extension.ToolRenderContext{Args: args})
			if got := strings.Join(component.Render(100), "\n"); !strings.Contains(got, "text-only does not support images; it saw a placeholder instead") {
				t.Fatalf("expanded=%t: missing warning in %q", expanded, got)
			}
		}
	}
}
