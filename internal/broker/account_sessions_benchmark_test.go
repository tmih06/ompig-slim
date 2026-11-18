package broker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

func BenchmarkSessionRouting(b *testing.B) {
	for _, tc := range []struct {
		provider string
		count    int
	}{{"fixture", 2}, {"fixture", 64}, {"codex-json", 2}, {"codex-json", 64}} {
		count := tc.count
		name := fmt.Sprintf("accounts=%d", count)
		if tc.provider == "codex-json" {
			name = fmt.Sprintf("codex/accounts=%d", count)
		}
		b.Run(name, func(b *testing.B) {
			client, host, traffic := delayedBroker(b, 0)
			var facts []ai.AccountSessionUsage
			for i := range count {
				key := fmt.Sprintf("account-%03d", i)
				credential := ai.Credential{Type: ai.CredentialAPIKey, Key: key}
				if tc.provider == "codex-json" {
					credential = ai.Credential{Type: ai.CredentialOAuth, Access: key, Expires: time.Now().Add(time.Hour).UnixMilli()}
					facts = append(facts, ai.AccountSessionUsage{
						AccountKey: &key, AccountKeySet: true, FetchedAt: time.Now().UnixMilli(), Plan: "free",
						Windows: []ai.AccountSessionUsageWindow{
							{ID: "primary", RemainingFraction: new(float64(i+1) / float64(count)), DurationMs: new((5 * time.Hour).Milliseconds())},
							{ID: "secondary", RemainingFraction: new(0.75), DurationMs: new((7 * 24 * time.Hour).Milliseconds())},
						},
					})
				}
				if _, err := host.vault.SaveAccount(b.Context(), tc.provider, &key, credential); err != nil {
					b.Fatal(err)
				}
			}
			router := ai.NewAccountRouter(&remoteStore{client: client})
			if len(facts) > 0 {
				router.SetSessionUsageSource(func(context.Context, string, string) ([]ai.AccountSessionUsage, error) {
					return facts, nil
				})
			}
			model := &ai.Model{ID: "fixture-model", ProviderMeta: ai.ProviderMetadata{ProviderID: tc.provider}}
			request := ai.AccountRouteRequest{SessionID: "sticky-session"}
			attempts := 0
			attempt := func(_ context.Context, selection ai.AccountSelection, _ ai.AccountObserve, _ bool) (ai.AssistantImages, error) {
				if selection.AccountKey == nil || *selection.AccountKey != fmt.Sprintf("account-%03d", count-1) {
					b.Fatalf("sticky assignment changed: %+v", selection)
				}
				attempts++
				return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
			}
			if _, err := router.ImagesRoutes(b.Context(), model, request, attempt); err != nil {
				b.Fatal(err)
			}
			before := traffic.requests.Load()
			attempts = 0
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := router.ImagesRoutes(b.Context(), model, request, attempt); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			if attempts != b.N {
				b.Fatalf("attempts=%d, want %d", attempts, b.N)
			}
			b.ReportMetric(float64(traffic.requests.Load()-before)/float64(b.N), "requests/op")
		})
	}
}
