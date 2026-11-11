package broker

import (
	"context"
	"fmt"
	"io"

	"github.com/MichaelKinsy/PiG/ai"
)

// runMigrate explicitly transfers local refresh ownership to the configured
// broker. Existing identities (including logout tombstones) are skipped. Local
// credentials remain untouched for recovery and are never implicitly uploaded.
func runMigrate(ctx context.Context, args []string, dir string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "--from-local" {
		return setupUsageError(stderr, "migrate requires --from-local")
	}
	saved, err := loadConnection(dir)
	if err != nil {
		fmt.Fprintln(stderr, "broker migrate:", err)
		return 1
	}
	client, err := newClient(dir, saved)
	if err != nil {
		fmt.Fprintln(stderr, "broker migrate:", err)
		return 1
	}
	store, err := ai.OpenLocalCredentialStore(dir)
	if err != nil {
		fmt.Fprintln(stderr, "broker migrate:", err)
		return 1
	}
	defer store.Close()
	accounts, err := store.ListAccounts(ctx)
	if err != nil {
		fmt.Fprintln(stderr, "broker migrate: local account listing failed")
		return 1
	}
	imported, skipped := 0, 0
	remote := &remoteStore{client: client}
	for _, account := range accounts {
		credential, err := store.ReadAccount(ctx, account.ProviderID, account.AccountKey)
		if err != nil {
			fmt.Fprintln(stderr, "broker migrate: local credential read failed")
			return 1
		}
		if credential == nil {
			continue
		}
		var inserted bool
		if err = remote.operation(ctx, vaultRequest{Operation: "import", Provider: account.ProviderID, AccountKey: account.AccountKey, Credential: credential}, &inserted); err != nil {
			fmt.Fprintln(stderr, "broker migrate:", err)
			return 1
		}
		if inserted {
			imported++
		} else {
			skipped++
		}
	}
	fmt.Fprintf(stdout, "Imported %d accounts; skipped %d existing identities. Local credentials were left unchanged.\n", imported, skipped)
	return 0
}
