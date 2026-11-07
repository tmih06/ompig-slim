package codingagent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestUsageStartsFastAccountBeforeSlowAccountFinishes(t *testing.T) {
	store := newUsageTestStore()
	store.add("test", new("slow"), ai.Credential{Type: ai.CredentialAPIKey, Key: "slow"})
	store.add("test", new("fast"), ai.Credential{Type: ai.CredentialAPIKey, Key: "fast"})
	release := make(chan struct{})
	defer close(release)
	slowStarted, fastStarted := make(chan struct{}), make(chan struct{})
	provider := &ai.UsageProvider{ID: "test", FetchUsage: func(ctx context.Context, params ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
		if params.Credential.APIKey == "slow" {
			close(slowStarted)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, context.Cause(ctx)
			}
		} else {
			close(fastStarted)
		}
		return usageReportFor("test"), nil
	}}
	service := usageTestService(store, provider)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); _, _ = service.FetchSnapshot(ctx, false) }()
	<-slowStarted
	select {
	case <-fastStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("the slow account blocked an independent account's request")
	}
	cancel()
	<-done
}

func TestUsageProgressPublishesFastAccountWhileSlowAccountIsPending(t *testing.T) {
	store := newUsageTestStore()
	store.add("test", new("slow"), ai.Credential{Type: ai.CredentialAPIKey, Key: "slow"})
	store.add("test", new("fast"), ai.Credential{Type: ai.CredentialAPIKey, Key: "fast"})
	release := make(chan struct{})
	defer close(release)
	provider := &ai.UsageProvider{ID: "test", FetchUsage: func(ctx context.Context, params ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
		if params.Credential.APIKey == "slow" {
			select {
			case <-release:
			case <-ctx.Done():
				return nil, context.Cause(ctx)
			}
		}
		return usageReportFor("test"), nil
	}}
	service := usageTestService(store, provider)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pushed := make(chan ai.UsageSnapshot, 2)
	done := make(chan error, 1)
	go func() {
		_, err := service.FetchSnapshotProgress(ctx, false, func(snapshot ai.UsageSnapshot) { pushed <- snapshot })
		done <- err
	}()
	select {
	case snapshot := <-pushed:
		if len(snapshot.Reports) != 1 || snapshot.Reports[0].Metadata["accountKey"] != "fast" || len(snapshot.UnavailableAccounts) != 0 {
			t.Fatalf("partial snapshot = %+v; pending accounts must not be unavailable", snapshot)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a fast quota report was hidden by a pending account")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled refresh succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not release its workers on cancellation")
	}
}

func TestUsageCachedAccountPublishesWhileAnotherAccountIsPending(t *testing.T) {
	store := newUsageTestStore()
	store.add("test", new("fast"), ai.Credential{Type: ai.CredentialAPIKey, Key: "fast"})
	release := make(chan struct{})
	defer close(release)
	var calls atomic.Int64
	provider := &ai.UsageProvider{ID: "test", FetchUsage: func(ctx context.Context, params ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
		if params.Credential.APIKey == "slow" {
			select {
			case <-release:
			case <-ctx.Done():
				return nil, context.Cause(ctx)
			}
		} else {
			calls.Add(1)
		}
		return usageReportFor("test"), nil
	}}
	service := usageTestService(store, provider)
	if _, err := service.FetchSnapshot(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	store.add("test", new("slow"), ai.Credential{Type: ai.CredentialAPIKey, Key: "slow"})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pushed := make(chan ai.UsageSnapshot, 2)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = service.FetchSnapshotProgress(ctx, false, func(snapshot ai.UsageSnapshot) { pushed <- snapshot })
	}()
	select {
	case snapshot := <-pushed:
		if len(snapshot.Reports) != 1 || calls.Load() != 1 {
			t.Fatalf("cached report was refetched: reports=%d calls=%d", len(snapshot.Reports), calls.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cached quota waited for the slow account")
	}
	cancel()
	<-done
}

func TestUsageReopenDoesNotJoinCanceledProbe(t *testing.T) {
	store := newUsageTestStore()
	store.add("test", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "fixture"})
	firstStarted, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	var calls atomic.Int64
	provider := &ai.UsageProvider{ID: "test", FetchUsage: func(ctx context.Context, _ ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-ctx.Done()
			<-release // Simulate transport cleanup still unwinding.
			return nil, context.Cause(ctx)
		}
		return usageReportFor("test"), nil
	}}
	service := usageTestService(store, provider)
	firstCtx, cancelFirst := context.WithCancel(t.Context())
	defer cancelFirst()
	firstDone := make(chan struct{})
	go func() { defer close(firstDone); _, _ = service.FetchSnapshot(firstCtx, false) }()
	<-firstStarted
	cancelFirst()
	<-firstDone
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	snapshot, err := service.FetchSnapshot(ctx, false)
	if err != nil || len(snapshot.Reports) != 1 || calls.Load() != 2 {
		t.Fatalf("reopen inherited a canceled request: reports=%d calls=%d err=%v", len(snapshot.Reports), calls.Load(), err)
	}
}

func TestUsageCachedUnsupportedAmbientRemainsOmitted(t *testing.T) {
	provider := &ai.UsageProvider{ID: "test", Supports: func(ai.UsageFetchParams) bool { return false }, FetchUsage: func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error) {
		t.Error("unsupported credentials reached the endpoint")
		return nil, nil
	}}
	service := NewUsageService(UsageServiceOptions{
		Providers: []*ai.UsageProvider{provider},
		GetAuth: func(context.Context, string, ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
			return &ai.AuthResult{Auth: ai.ModelAuth{APIKey: "fixture"}}, nil
		},
	})
	for range 2 {
		snapshot, err := service.FetchSnapshot(t.Context(), false)
		if err != nil || len(snapshot.UnavailableAccounts) != 0 || len(snapshot.Reports) != 0 {
			t.Fatalf("unsupported ambient credential shown as a failed account: %+v err=%v", snapshot, err)
		}
	}
}

func TestUsageCachedOAuthQuotaDoesNotWaitForTokenRefresh(t *testing.T) {
	now := time.Now()
	store := newUsageTestStore()
	store.add("test", nil, ai.Credential{Type: ai.CredentialOAuth, Access: "fixture", AccountID: "account", Expires: now.Add(6 * time.Minute).UnixMilli()})
	var refreshes atomic.Int64
	service := usageTestService(store, &ai.UsageProvider{ID: "test", FetchUsage: func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error) {
		return usageReportFor("test"), nil
	}})
	service.options.Now = func() time.Time { return now }
	service.options.GetAuth = func(context.Context, string, ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
		refreshes.Add(1)
		return nil, nil
	}
	if _, err := service.FetchSnapshot(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute) // Token is near expiry; the quota is still fresh.
	snapshot, err := service.FetchSnapshot(t.Context(), false)
	if err != nil || len(snapshot.Reports) != 1 || refreshes.Load() != 0 {
		t.Fatalf("fresh quota waited for OAuth: reports=%d refreshes=%d err=%v", len(snapshot.Reports), refreshes.Load(), err)
	}
	if _, err := service.FetchSnapshot(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if refreshes.Load() != 1 {
		t.Fatal("forced refresh did not renew the near-expiry token")
	}
}
