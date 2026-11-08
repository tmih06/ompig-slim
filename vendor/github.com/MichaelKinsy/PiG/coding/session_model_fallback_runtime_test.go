package coding

import (
	"context"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

// TestFallbackCooldownRestore verifies a prompt restores primary only after the
// provider's reset and never when the user explicitly chose never-revert.
func TestFallbackCooldownRestore(t *testing.T) {
	for _, policy := range []string{"cooldown-expiry", "never"} {
		t.Run(policy, func(t *testing.T) {
			s := roleTestSession(t, map[string][]string{"review": {"openai/gpt-4o-mini:off"}})
			now := time.Unix(2000000000, 0)
			s.fallbackNow = func() time.Time { return now }
			if err := s.SettingsManager().UpdateGlobal(func(settings *Settings) { settings.Retry.FallbackRevertPolicy = policy }); err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			message := appendRoleFailure(t, s, "quota exceeded; try again in 120 seconds")
			hopped, err := s.tryModelFallback(context.Background(), message)
			s.mu.Unlock()
			if err != nil || !hopped {
				t.Fatal(hopped, err)
			}
			if err = s.restoreFallbackPrimary(context.Background()); err != nil || s.Model().ID != "gpt-4o-mini" {
				t.Fatal("early restore", s.Model().ID, err)
			}
			now = now.Add(121 * time.Second)
			if err = s.restoreFallbackPrimary(context.Background()); err != nil {
				t.Fatal(err)
			}
			want := "gpt-4o"
			if policy == "never" {
				want = "gpt-4o-mini"
			}
			if s.Model().ID != want {
				t.Fatal(s.Model().ID, want)
			}
		})
	}
}

// TestFallbackChainAtTailProtectsItself makes a second model-keyed chain reachable.
func TestFallbackChainAtTailProtectsItself(t *testing.T) {
	s := roleTestSession(t, map[string][]string{"review": {"openai/gpt-4o-mini"}, "openai/gpt-4o-mini": {"openai/gpt-4.1"}})
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, want := range []string{"gpt-4o-mini", "gpt-4.1"} {
		message := appendRoleFailure(t, s, "billing quota exceeded")
		hopped, err := s.tryModelFallback(context.Background(), message)
		if err != nil || !hopped || s.Model().ID != want {
			t.Fatal(hopped, err, s.Model().ID, want)
		}
	}
}

// TestMissingCredentialPrewalk restores the primary as soon as its credential
// returns, without inventing a failed assistant response or waiting a cooldown.
func TestMissingCredentialPrewalk(t *testing.T) {
	s := roleTestSession(t, map[string][]string{"review": {"google/gemini-2.5-flash:off"}})
	t.Setenv("GEMINI_API_KEY", "fixture-gemini-key")
	t.Setenv("OPENAI_API_KEY", "")
	if err := s.prepareFallbackModel(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Model().ID != "gemini-2.5-flash" {
		t.Fatal("missing credential did not prewalk", s.Model().ID)
	}
	t.Setenv("OPENAI_API_KEY", "fixture-restored-key")
	if err := s.prepareFallbackModel(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Model().ID != "gpt-4o" {
		t.Fatal("credential return did not restore", s.Model().ID)
	}
}

// TestSignedThinkingSiblingDeferral matches OMP's signer retry rule and its
// rateLimitWalksChain override; unrelated-provider fallbacks remain eligible.
func TestSignedThinkingSiblingDeferral(t *testing.T) {
	s := roleTestSession(t, nil)
	source := &ai.Model{ID: "signer", ProviderMeta: ai.ProviderMetadata{ProviderID: "anthropic", API: ai.APIAnthropicMessages}}
	sibling := &ai.Model{ID: "sibling", ProviderMeta: ai.ProviderMetadata{ProviderID: "anthropic", API: ai.APIAnthropicMessages}}
	s.agent.SetModel(source)
	signed := &agent.AssistantMessage{Role: "assistant", Provider: "anthropic", ModelID: "signer", API: ai.APIAnthropicMessages, Content: []ai.AssistantContentBlock{ai.ThinkingContent{Thinking: "private", ThinkingSignature: "signed"}}}
	failed := &agent.AssistantMessage{Role: "assistant", Provider: "anthropic", ModelID: "signer", ErrorMessage: "429 rate limit"}
	s.agent.SetMessages([]agent.AgentMessage{{Assistant: signed}, {Assistant: failed}})
	if !s.deferSignedThinkingFallback(failed, sibling) {
		t.Fatal("signer retry not preferred")
	}
	if err := s.SettingsManager().UpdateGlobal(func(settings *Settings) { settings.Retry.RateLimitWalksChain = new(true) }); err != nil {
		t.Fatal(err)
	}
	if s.deferSignedThinkingFallback(failed, sibling) {
		t.Fatal("rate limit override ignored")
	}
}

// TestFallbackRetryAfterLongest honors provider milliseconds and longer reset text.
func TestFallbackRetryAfterLongest(t *testing.T) {
	for _, tc := range []struct {
		text string
		want time.Duration
	}{{"retry-after-ms: 2500", 2500 * time.Millisecond}, {"retry-after: 2; try again in 3 minutes", 3 * time.Minute}, {"unknown", 5 * time.Minute}} {
		if got := fallbackCooldown(tc.text); got != tc.want {
			t.Fatalf("%s: %v want %v", tc.text, got, tc.want)
		}
	}
}

// TestUsageQuotaHealthEvidence excludes unknown/future-stale windows and respects
// exact model scope without treating spending credits as coding-plan quota.
func TestUsageQuotaHealthEvidence(t *testing.T) {
	s := roleTestSession(t, nil)
	remaining := .05
	now := time.Now().UnixMilli()
	report := ai.UsageReport{Provider: "openai", FetchedAt: now, Limits: []ai.UsageLimit{{ID: "5h", Amount: ai.UsageAmount{RemainingFraction: &remaining}}}}
	if percent, known := modelReportRemaining(report, s.Model(), now); !known || percent != 5 {
		t.Fatal(percent, known)
	}
	report.Limits[0].Scope.ModelID = "another-model"
	if _, known := modelReportRemaining(report, s.Model(), now); known {
		t.Fatal("foreign model quota used")
	}
	report.Limits[0].Scope.ModelID = ""
	report.Limits[0].ID = "credit balance"
	if _, known := modelReportRemaining(report, s.Model(), now); known {
		t.Fatal("billing credit used")
	}
}
