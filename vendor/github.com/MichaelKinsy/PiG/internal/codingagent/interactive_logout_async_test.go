package codingagent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

type blockingLogoutCredentialStore struct {
	ai.ApplicationCredentialStore
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingLogoutCredentialStore) List(ctx context.Context) ([]ai.CredentialInfo, error) {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *blockingLogoutCredentialStore) Close() error { return nil }

func TestInteractiveLogoutReturnsWhileCredentialListIsSlow(t *testing.T) {
	previous := ai.CredentialStoreFactory
	t.Cleanup(func() { ai.CredentialStoreFactory = previous })
	store := &blockingLogoutCredentialStore{entered: make(chan struct{}), release: make(chan struct{})}
	ai.CredentialStoreFactory = func(string) (ai.ApplicationCredentialStore, error) { return store, nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mode := &InteractiveMode{runCtx: ctx, opts: InteractiveModeOptions{AgentDir: t.TempDir()}}
	start := time.Now()
	mode.startInteractiveLogout(ctx)
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("/logout blocked the caller for %s behind credential storage", elapsed)
	}
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("logout did not start credential metadata loading")
	}
	close(store.release)
	cancel()
	mode.backgroundTasks.Wait()
}
