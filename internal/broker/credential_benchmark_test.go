package broker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// BenchmarkBrokerCredentialPaths separates SQLite work from live HTTP
// snapshot and metadata reads. Eight accounts represent a personal pool; 64
// exercise scaling. Each pool shares one provider and mixes API keys and OAuth,
// without network refreshes or real secrets. No snapshot caching is permitted.
func BenchmarkBrokerCredentialPaths(b *testing.B) {
	for _, count := range []int{8, 64} {
		b.Run(fmt.Sprintf("accounts=%d", count), func(b *testing.B) {
			client, _, server := testClients(b)
			host := server.Config.Handler.(*Store)
			remote := &remoteStore{client: client}
			ctx := context.Background()
			for i := range count {
				key := fmt.Sprintf("account-%d", i)
				credential := ai.Credential{Type: ai.CredentialAPIKey, Key: key}
				if i%2 == 0 {
					credential = ai.Credential{Type: ai.CredentialOAuth, Access: key, Refresh: "private-" + key, Expires: time.Now().Add(time.Hour).UnixMilli()}
				}
				if _, err := host.vault.SaveAccount(ctx, "fixture", &key, credential); err != nil {
					b.Fatal(err)
				}
			}
			for _, operation := range []struct {
				name string
				run  func() error
			}{
				{"entries", func() error { _, err := host.entries(ctx); return err }},
				{"snapshot", func() error { _, err := remote.snapshot(ctx); return err }},
				{"count", func() error { _, err := remote.AccountCount(); return err }},
				{"hasProvider", func() error { _, err := remote.HasProvider(ctx, "fixture"); return err }},
			} {
				b.Run(operation.name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if err := operation.run(); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
