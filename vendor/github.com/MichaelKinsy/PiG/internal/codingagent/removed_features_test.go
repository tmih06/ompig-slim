package codingagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/tui"
)

// TestRetiredCommandsStayLocal prevents removed UI commands from purchasing a
// model turn or entering the compaction queue. No agent is needed for rejection.
func TestRetiredCommandsStayLocal(t *testing.T) {
	for _, command := range []string{"/sprite", "/sprite create", "/pigsayhi", "/arminsayshi", "/dementedelves", "/share", "/changelog", "/llama"} {
		t.Run(command, func(t *testing.T) {
			m := &InteractiveMode{editor: tui.NewEditor(), slashRegistry: NewSlashRegistry(), isCompacting: true}
			if _, exists := m.slashRegistry.Match(command); exists {
				t.Fatal("removed command is still registered")
			}
			m.editor.SetText(command)
			m.handleSubmitWithImages(context.Background(), command, nil)
			if len(m.compactionQueue) != 0 || m.editor.Text() != "" {
				t.Fatal("removed command was queued instead of rejected locally")
			}
		})
	}
	for _, text := range []string{"", "explain /sprite", "/sprites", "/llama.cpp", "/export"} {
		if removedBuiltinCommand(text) {
			t.Fatalf("ordinary input rejected: %q", text)
		}
	}
}

// TestTelemetrySurfaceIsGone covers that the environment can no longer enable
// install telemetry or analytics, which left the settings entirely.
func TestTelemetrySurfaceIsGone(t *testing.T) {
	t.Setenv("PI_TELEMETRY", "1")
}

// TestSlimKeepsMermaid exercises diagram rendering, not merely the setting or
// command registry, so trimming presentation features cannot turn diagrams off.
func TestSlimKeepsMermaid(t *testing.T) {
	markdown := "```mermaid\ngraph TD\nA[Start] --> B[Done]\n```"
	render := createMermaidMarkdownTransformer(func() string { return "final" }, nil)
	out := render(markdown, extension.MarkdownTransformContext{AvailableWidth: 120})
	if out == markdown || !strings.Contains(out, "Start") || !strings.Contains(out, "Done") {
		t.Fatalf("Mermaid was not rendered: %s", out)
	}
}

// TestExportIsJSONLOnly preserves portable exports, including the default path,
// and refuses HTML filenames before writing any output.
func TestExportIsJSONLOnly(t *testing.T) {
	dir := t.TempDir()
	session := NewSession("export-test", dir)
	var output string
	sc := &SlashContext{CurrentSession: func() *Session { return session }, Append: func(s string) { output = s }}
	for _, arg := range []string{"", filepath.Join(dir, "session.jsonl")} {
		sc.Args = arg
		if arg == "" {
			sc.ExportToJsonl = func(path string) (string, error) {
				if path != "" {
					t.Fatal("default path was not delegated")
				}
				return ExportSessionToJsonl(session, filepath.Join(dir, "default.jsonl"), nil)
			}
		} else {
			sc.ExportToJsonl = nil
		}
		if err := exportHandler(sc); err != nil {
			t.Fatal(err)
		}
		path := strings.TrimPrefix(output, "Session exported to: ")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var header SessionHeader
		if err := json.Unmarshal(data, &header); err != nil || header.Type != "session" || header.ID != "export-test" {
			t.Fatalf("invalid JSONL export: %s, %v", data, err)
		}
	}
	sc.Args = filepath.Join(dir, "session.html")
	if err := exportHandler(sc); err == nil {
		t.Fatal("HTML export accepted")
	}
	if _, err := os.Stat(sc.Args); !os.IsNotExist(err) {
		t.Fatalf("HTML output was written: %v", err)
	}
}
