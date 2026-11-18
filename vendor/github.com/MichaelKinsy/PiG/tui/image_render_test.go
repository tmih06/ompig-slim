package tui

import (
	"strings"
	"testing"
)

func TestHerdrNeedsGraphicsSignalForKittyImages(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "herdr")
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_KITTY_GRAPHICS", "")

	if caps := detectCapabilitiesFromEnvironment(nil, "linux"); caps.Images != "" {
		t.Fatalf("images enabled without Herdr graphics signal: %q", caps.Images)
	}

	t.Setenv("HERDR_KITTY_GRAPHICS", "1")
	if caps := detectCapabilitiesFromEnvironment(nil, "linux"); caps.Images != ImageProtocolKitty {
		t.Fatalf("images not enabled with Herdr graphics signal: %q", caps.Images)
	}
}

func TestToolImageShowsFallbackWithoutImageProtocol(t *testing.T) {
	SetCapabilities(TerminalCapabilities{})
	t.Cleanup(func() { SetCapabilities(DetectCapabilities(nil)) })

	component := NewToolExecutionComponent("read", "call", nil, ToolExecutionOptions{}, nil, nil, "")
	component.UpdateResult(ToolResultUpdate{Content: []ToolResultContent{{Type: "image", Data: "aGVsbG8=", MimeType: "image/png"}}})

	rows := strings.Join(component.Render(80), "\n")
	if !strings.Contains(rows, "[Image: [image/png]") {
		t.Fatalf("image result disappeared without a terminal image protocol:\n%s", rows)
	}
}
