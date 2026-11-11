package broker

import (
	"context"
	"testing"
)

// BenchmarkSyncFilesUnchanged measures a five-document idle poll against a real
// loopback broker, including local ancestry persistence but excluding setup.
func BenchmarkSyncFilesUnchanged(b *testing.B) {
	client, _, _ := testClients(b)
	ctx := context.Background()
	if err := client.SyncFiles(ctx); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := client.SyncFiles(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
