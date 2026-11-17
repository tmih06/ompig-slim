package broker

import (
	"context"
	"fmt"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

func BenchmarkSessionRouting(b *testing.B) {
	for _, count := range []int{2, 64} {
		b.Run(fmt.Sprintf("accounts=%d", count), func(b *testing.B) {
			client, host, traffic := delayedBroker(b, 0)
			for i := range count {
				key := fmt.Sprintf("account-%03d", i)
				if _, err := host.vault.SaveAccount(b.Context(), "fixture", &key, ai.Credential{Type: ai.CredentialAPIKey, Key: key}); err != nil {
					b.Fatal(err)
				}
			}
			router := ai.NewAccountRouter(&remoteStore{client: client})
			model := &ai.Model{ID: "fixture-model", ProviderMeta: ai.ProviderMetadata{ProviderID: "fixture"}}
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
