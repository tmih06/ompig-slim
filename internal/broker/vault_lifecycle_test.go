package broker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestVaultCloseWaitsForRefresh protects a rotation when HTTP shutdown times out:
// Close must wait for admitted provider work before destroying SQLite's handle.
func TestVaultCloseWaitsForRefresh(t *testing.T) {
	client, _, server := testClients(t)
	host := server.Config.Handler.(*Store)
	remote := &remoteStore{client: client}
	started, release := make(chan struct{}), make(chan struct{})
	registerRefresh(t, func(ctx context.Context, c ai.OAuthCredentials) (ai.OAuthCredentials, error) {
		close(started)
		<-release
		c.Refresh = "committed-before-close"
		c.Access = "fresh"
		c.Expires = time.Now().Add(time.Hour).UnixMilli()
		return c, nil
	})
	ctx := context.Background()
	if _, err := remote.SaveAccount(ctx, "broker-fixture", nil, ai.Credential{Type: ai.CredentialOAuth, Access: "old", Refresh: "old-private", Expires: 1}); err != nil {
		t.Fatal(err)
	}
	expected, err := remote.Read(ctx, "broker-fixture")
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := remote.RefreshOAuth(ctx, "broker-fixture", nil, true, *expected); result <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("exchange never started")
	}
	closed := make(chan error, 1)
	go func() { closed <- host.Close() }()
	select {
	case err := <-closed:
		close(release)
		t.Fatal("closed vault during provider exchange", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	reopened := openStore(t, host.dir)
	credential, err := reopened.ReadAccount(ctx, "broker-fixture", nil)
	if err != nil || credential == nil || credential.Refresh != "committed-before-close" {
		t.Fatal("shutdown lost rotation", err)
	}
}

// TestInvalidConnectionCannotOpenLocalVault keeps malformed or missing bootstrap
// secrets from activating old refresh tokens. An unrelated profile stays local.
func TestInvalidConnectionCannotOpenLocalVault(t *testing.T) {
	previous := ai.CredentialStoreFactory
	t.Cleanup(func() { ai.CredentialStoreFactory = previous })
	dir := t.TempDir()
	t.Setenv(EnvURL, "https://broker.example")
	t.Setenv(EnvToken, "")
	t.Setenv(EnvTokenFile, "")
	if _, err := Connect(context.Background(), dir); err == nil {
		t.Fatal("accepted missing secret")
	}
	if _, err := ai.OpenCredentialStore(dir); err == nil {
		t.Fatal("invalid broker fell back to local vault")
	}
	other := t.TempDir()
	local, err := ai.OpenCredentialStore(other)
	if err != nil {
		t.Fatal("unrelated profile was made remote", err)
	}
	local.Close()
	if _, err := os.Stat(filepath.Join(dir, "agent.db")); !os.IsNotExist(err) {
		t.Fatal("created local vault for broken broker", err)
	}
}
