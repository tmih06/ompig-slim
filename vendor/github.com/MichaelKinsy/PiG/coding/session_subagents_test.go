package coding

import (
	"slices"
	"testing"

	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/subagents"
)

func TestBuiltinSubagentsToggleAndToolSelection(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	cwd, dir := t.TempDir(), t.TempDir()
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{CWD: cwd, AgentDir: dir, SettingsManager: icodingagent.NewInMemorySettingsManager(Settings{})})
	if err != nil {
		t.Fatal(err)
	}
	defer svcs.Close()
	for _, test := range []struct {
		name string
		opts SessionOptions
		want bool
	}{{"default", SessionOptions{}, true}, {"explicit allowlist", SessionOptions{AllowedTools: map[string]struct{}{"read": {}}}, false}, {"no tools", SessionOptions{NoTools: "all"}, false}, {"no builtins", SessionOptions{SkipBuiltinTools: true}, false}, {"excluded", SessionOptions{ExcludedTools: map[string]struct{}{"Agent": {}}}, false}} {
		t.Run(test.name, func(t *testing.T) {
			if err := subagents.SaveSettings(cwd, map[string]any{"enabled": false}); err != nil {
				t.Fatal(err)
			}
			opts := test.opts
			opts.NoSession = true
			opts.ResourceLoader = NoResources
			s, err := NewSession(svcs, opts)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if slices.Contains(s.ActiveToolNames(), "Agent") {
				t.Fatal("disabled Agent exposed")
			}
			out, err := s.DispatchSlash("/subagents on")
			if err != nil || len(out) == 0 {
				t.Fatalf("toggle on failed: %v %v", out, err)
			}
			if got := slices.Contains(s.ActiveToolNames(), "Agent"); got != test.want {
				t.Fatalf("Agent exposed=%t; want %t, tools=%v", got, test.want, s.ActiveToolNames())
			}
			if _, err = s.DispatchSlash("/subagents off"); err != nil {
				t.Fatal(err)
			}
			for _, name := range s.ActiveToolNames() {
				if subagents.IsTool(name) {
					t.Fatal("disabled tool remains callable", name)
				}
			}
		})
	}
}
