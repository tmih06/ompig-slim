package broker

import (
	"context"
	"fmt"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
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

// BenchmarkExportCredentials measures the snapshot used by every idle reconcile
// and mutation, with eight accounts sharing one provider's explicit selection.
func BenchmarkExportCredentials(b *testing.B) {
	store := openStore(b, b.TempDir())
	ctx := context.Background()
	for i := range 8 {
		key := fmt.Sprintf("account-%d", i)
		if _, err := store.SaveAccount(ctx, "fixture", &key, ai.Credential{Type: ai.CredentialAPIKey, Key: key}); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := store.ExportCredentials(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
