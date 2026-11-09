package coding

import (
	"context"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	_ "github.com/MichaelKinsy/PiG/ai/all"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// roleTestSession builds a real session with fixture-only credentials; tests never call a provider.
func roleTestSession(t *testing.T, chains map[string][]string) *Session {
	t.Helper()
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "fixture-not-a-real-key")
	settings := Settings{Config: modelroles.Config{ModelRoles: map[string]string{"default": "openai/gpt-4o", "review": "openai/gpt-4o"}, CycleOrder: []string{"review", "default"}}, Retry: &icodingagent.RetrySettingsJSON{FallbackChains: chains}}
	sm := NewInMemorySettingsManager(settings)
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: sm})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { services.Close() })
	model, err := BuildModel("@review", services)
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewSession(services, SessionOptions{Model: model, ModelRole: "review", NoSession: true, NoTools: "all", ResourceLoader: NoResources})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// appendRoleFailure reproduces a persisted failed response before post-run recovery.
func appendRoleFailure(t *testing.T, s *Session, text string) *agent.AssistantMessage {
	t.Helper()
	m := &agent.AssistantMessage{Role: "assistant", Provider: "openai", ModelID: s.Model().ID, StopReason: ai.StopReasonError, ErrorMessage: text}
	msg := agent.AgentMessage{Assistant: m}
	if _, err := s.inner.AppendMessage(msg); err != nil {
		t.Fatal(err)
	}
	s.agent.SetMessages(s.inner.BuildContext(nil))
	return m
}

// TestRoleFallbackRecovery checks actual model mutation, chain exhaustion, and logical identity.
func TestRoleFallbackRecovery(t *testing.T) {
	s := roleTestSession(t, map[string][]string{"review": {"missing/no-model", "openai/gpt-4o-mini:off", "openai/gpt-4o"}, "default": {"openai/gpt-4.1"}})
	s.mu.Lock()
	defer s.mu.Unlock()
	message := appendRoleFailure(t, s, "billing quota exceeded")
	hopped, err := s.tryModelFallback(context.Background(), message)
	if err != nil || !hopped || s.Model().ID != "gpt-4o-mini" || s.ActiveModelRole() != "review" || s.ThinkingLevel() != ai.ThinkingOff {
		t.Fatalf("hop=%v err=%v model=%v role=%s effort=%s", hopped, err, s.Model().ID, s.ActiveModelRole(), s.ThinkingLevel())
	}
	if got := branchModelRole(s.inner.GetContextBranch(), s.Model()); got != "review" {
		t.Fatalf("fallback lost persisted role: %q", got)
	}
	message = appendRoleFailure(t, s, "billing quota exceeded")
	if hopped, err = s.tryModelFallback(context.Background(), message); err != nil || hopped {
		t.Fatalf("chain wrapped: %v %v", hopped, err)
	}
}

// TestRoleEmptyChainAndOverflow ensures another role sharing a model cannot steal recovery.
func TestRoleEmptyChainAndOverflow(t *testing.T) {
	for _, tc := range []struct {
		name, errorText string
		chain           []string
	}{{"empty", "billing quota exceeded", []string{}}, {"overflow", "maximum context length exceeded", []string{"openai/gpt-4o-mini"}}} {
		t.Run(tc.name, func(t *testing.T) {
			s := roleTestSession(t, map[string][]string{"review": tc.chain, "default": {"openai/gpt-4o-mini"}})
			s.mu.Lock()
			defer s.mu.Unlock()
			message := appendRoleFailure(t, s, tc.errorText)
			if hopped, err := s.tryModelFallback(context.Background(), message); err != nil || hopped {
				t.Fatal(hopped, err)
			}
		})
	}
}

// TestRoleFallbackPolicy makes the independent enable and trigger settings effective.
func TestRoleFallbackPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, trigger, errorText string
		disabled                 bool
	}{{"disabled", "any", "billing quota exceeded", true}, {"usage only", "usage-limit", "network error", false}, {"except usage", "except-usage-limit", "billing quota exceeded", false}} {
		t.Run(tc.name, func(t *testing.T) {
			s := roleTestSession(t, map[string][]string{"review": {"openai/gpt-4o-mini"}})
			if err := s.SettingsManager().UpdateGlobal(func(settings *Settings) {
				settings.Retry.ModelFallback = new(!tc.disabled)
				settings.Retry.FallbackOn = tc.trigger
			}); err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			message := appendRoleFailure(t, s, tc.errorText)
			if hopped, err := s.tryModelFallback(context.Background(), message); err != nil || hopped {
				t.Fatal(hopped, err)
			}
		})
	}
}

// TestRoleCycleRemovedScopes reproduces an old scoped-N cycle after its roles
// were removed and Default/Smol reassigned. Both directions switch real models,
// while intentional empty cycles remain disabled and reads never rewrite settings.
func TestRoleCycleRemovedScopes(t *testing.T) {
	s := roleTestSession(t, nil)
	if err := s.SettingsManager().UpdateGlobal(func(settings *Settings) {
		settings.ModelRoles = map[string]string{"default": "openai/gpt-4o:off", "smol": "openai/gpt-4o-mini:off"}
		settings.CycleOrder = []string{"scoped-1", "scoped-2", "scoped-3", "scoped-4"}
	}); err != nil {
		t.Fatal(err)
	}
	for _, direction := range []string{"forward", "backward"} {
		for _, want := range []string{"smol", "default", "smol", "default"} {
			result, err := s.CycleModel(direction)
			if err != nil || result == nil || s.ActiveModelRole() != want {
				t.Fatalf("%s: result=%v err=%v role=%s want=%s", direction, result, err, s.ActiveModelRole(), want)
			}
			id := "gpt-4o"
			if want == "smol" {
				id = "gpt-4o-mini"
			}
			if s.Model().ID != id || s.ThinkingLevel() != ai.ThinkingOff {
				t.Fatalf("model=%s thinking=%s", s.Model().ID, s.ThinkingLevel())
			}
		}
	}
	if got := s.SettingsManager().Get().CycleOrder; len(got) != 4 || got[0] != "scoped-1" {
		t.Fatal("cycling rewrote saved settings", got)
	}
	if err := s.SettingsManager().SetRoleCycleOrder([]string{}, "global"); err != nil {
		t.Fatal(err)
	}
	if result, err := s.CycleModel("forward"); err != nil || result != nil {
		t.Fatalf("disabled cycle switched: %v %v", result, err)
	}
}

// TestRoleCycleSharedModel verifies role identity is not inferred from physical model equality.
func TestRoleCycleSharedModel(t *testing.T) {
	s := roleTestSession(t, nil)
	for _, want := range []string{"default", "review"} {
		result, err := s.CycleModel("forward")
		if err != nil || result == nil || s.ActiveModelRole() != want {
			t.Fatalf("result=%v err=%v role=%s want=%s", result, err, s.ActiveModelRole(), want)
		}
	}
}
