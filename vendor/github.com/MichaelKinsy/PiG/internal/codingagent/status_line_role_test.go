package codingagent

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// roleFooterModel is a fixture chat model with a reasoning ladder, so the footer
// renders its effort suffix as well as its name.
func roleFooterModel() *ai.Model {
	return &ai.Model{
		ID:           "claude-sonnet-4",
		ProviderMeta: ai.ProviderMetadata{ProviderID: "anthropic", Reasoning: true},
		Capabilities: ai.ModelCapabilities{MaxThinking: "high", ContextWindow: 200000},
	}
}

// footerSelectionLine returns line 2 of the footer, where the model line renders.
func footerSelectionLine(t *testing.T, role string) string {
	t.Helper()
	footer := NewFooterComponent(roleFooterModel(), "agent", nil)
	footer.SetThinkingLevel("high")
	footer.activeRole = func() string { return role }
	lines := footer.Render(120)
	if len(lines) < 2 {
		t.Fatalf("footer rendered %d lines, want at least 2", len(lines))
	}
	return widthx.StripTerminalSequences(lines[1])
}

// TestFooterShowsActiveRoleBesideModel covers the reason the role is displayed at
// all: Ctrl+P cycling and fallback recovery can leave two different roles on the
// same physical model, so the model name alone cannot say which one is active.
func TestFooterShowsActiveRoleBesideModel(t *testing.T) {
	for _, role := range []string{"default", "review", "my-custom_role"} {
		line := footerSelectionLine(t, role)
		want := "@" + role + " claude-sonnet-4 • high"
		if !strings.Contains(line, want) {
			t.Fatalf("role %q not shown beside the model: %q", role, line)
		}
	}
	// Two roles that resolve to one model must be distinguishable.
	if first, second := footerSelectionLine(t, "default"), footerSelectionLine(t, "review"); first == second {
		t.Fatalf("shared-model roles render identically: %q", first)
	}
}

// TestFooterOmitsRoleWhenUnset keeps a session without role identity byte-identical
// to upstream: a direct model selection and a virtual model render no role.
func TestFooterOmitsRoleWhenUnset(t *testing.T) {
	line := footerSelectionLine(t, "")
	if !strings.Contains(line, "claude-sonnet-4 • high") {
		t.Fatalf("model line missing: %q", line)
	}
	if strings.Contains(line, "@") {
		t.Fatalf("unset role rendered a marker: %q", line)
	}
	// A footer never given a session has no role source at all.
	footer := NewFooterComponent(roleFooterModel(), "agent", nil)
	lines := footer.Render(120)
	for _, candidate := range lines {
		if strings.Contains(widthx.StripTerminalSequences(candidate), "@") {
			t.Fatalf("unbound footer rendered a role: %q", candidate)
		}
	}
}

// TestFooterFollowsRoleChangesLive pins the read-on-render contract: a role can
// change without the physical model changing, so a stale cached value would show
// the wrong role until an unrelated model rebind.
func TestFooterFollowsRoleChangesLive(t *testing.T) {
	footer := NewFooterComponent(roleFooterModel(), "agent", nil)
	role := "default"
	footer.activeRole = func() string { return role }
	if line := widthx.StripTerminalSequences(footer.Render(120)[1]); !strings.Contains(line, "@default") {
		t.Fatalf("initial role missing: %q", line)
	}
	role = "review"
	if line := widthx.StripTerminalSequences(footer.Render(120)[1]); !strings.Contains(line, "@review") {
		t.Fatalf("role change not reflected: %q", line)
	}
}
