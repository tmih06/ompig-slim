package ai_test

// Structured credential decoding must keep bearer material, cookie state, and
// regional endpoints separate; serialized credential JSON must never become a
// request's bearer token.

import (
	"context"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)


// TestAlibabaTokenPlanSerializedCredential pins the region-locked credential:
// the stored JSON keeps the token, cookie and region, and a bare key is its own
// credential. The api-key method decodes both, so the raw serialized value never
// becomes bearer material.
func TestAlibabaTokenPlanSerializedCredential(t *testing.T) {
	auth, err := ai.BuiltinProviderAuth("alibaba-token-plan")
	if err != nil || auth.APIKey == nil {
		t.Fatalf("alibaba-token-plan auth = %+v, %v", auth, err)
	}
	ctx := context.Background()

	// A bare international key: the token is the bearer and no region overrides the catalog endpoint.
	t.Setenv("ALIBABA_TOKEN_PLAN_API_KEY", "")
	resolved, err := ai.ResolveProviderAuth(ctx, "alibaba-token-plan", auth, emptyCredentialStore{}, ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{})
	if err != nil {
		t.Fatalf("no-key resolve error: %v", err)
	}
	if resolved != nil {
		t.Fatalf("resolve without a key = %+v, want nil", resolved)
	}

	t.Setenv("ALIBABA_TOKEN_PLAN_API_KEY", "sk-test-key")
	resolved, err = ai.ResolveProviderAuth(ctx, "alibaba-token-plan", auth, emptyCredentialStore{}, ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{})
	if err != nil {
		t.Fatalf("env resolve error: %v", err)
	}
	if resolved == nil || resolved.Auth.APIKey != "sk-test-key" || resolved.Auth.BaseURL != "" {
		t.Fatalf("env resolve = %+v, want the bare key and no region", resolved)
	}

	// A stored structured credential: the token is the bearer and the region replaces the endpoint.
	const stored = `{"token":"sk-test-key","cookie":"c=1","baseUrl":"https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1"}`
	resolved, err = ai.ResolveProviderAuth(ctx, "alibaba-token-plan", auth, credentialStoreWith("alibaba-token-plan", stored), ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{})
	if err != nil {
		t.Fatalf("stored resolve error: %v", err)
	}
	if resolved == nil || resolved.Auth.APIKey != "sk-test-key" {
		t.Fatalf("stored resolve = %+v, want the decoded token", resolved)
	}
	if resolved.Auth.BaseURL != "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("stored base URL = %q, want the China region", resolved.Auth.BaseURL)
	}

	// An unparsable credential is refused instead of being sent as a bearer token.
	if _, err := ai.ResolveProviderAuth(ctx, "alibaba-token-plan", auth, credentialStoreWith("alibaba-token-plan", "not-a-key"), ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{}); err == nil {
		t.Fatal("resolve of an invalid credential = nil error, want a configuration error")
	}
}

// Alibaba Coding Plan remembers its region in the stored key as
// `{token, enterpriseUrl}`, which the api-key method decodes the same way.
func TestAlibabaCodingPlanStructuredCredential(t *testing.T) {
	auth, err := ai.BuiltinProviderAuth("alibaba-coding-plan")
	if err != nil || auth.APIKey == nil {
		t.Fatalf("alibaba-coding-plan auth = %+v, %v", auth, err)
	}
	const stored = `{"token":"sk-china-key","enterpriseUrl":"https://coding.dashscope.aliyuncs.com/v1"}`
	resolved, err := ai.ResolveProviderAuth(context.Background(), "alibaba-coding-plan", auth, credentialStoreWith("alibaba-coding-plan", stored), ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{})
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if resolved == nil || resolved.Auth.APIKey != "sk-china-key" || resolved.Auth.BaseURL != "https://coding.dashscope.aliyuncs.com/v1" {
		t.Fatalf("resolve = %+v, want the token and the China endpoint", resolved)
	}

	// A bare key stays itself and keeps the catalog endpoint.
	resolved, err = ai.ResolveProviderAuth(context.Background(), "alibaba-coding-plan", auth, credentialStoreWith("alibaba-coding-plan", "sk-intl-key"), ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{})
	if err != nil {
		t.Fatalf("bare resolve error: %v", err)
	}
	if resolved == nil || resolved.Auth.APIKey != "sk-intl-key" || resolved.Auth.BaseURL != "" {
		t.Fatalf("bare resolve = %+v, want the bare key and no region", resolved)
	}
}

// credentialStoreWith is a CredentialStore whose only credential is provider's
// stored api key.
func credentialStoreWith(provider, key string) ai.CredentialStore {
	return providerCredentialStore{provider: provider, credential: ai.Credential{Type: ai.CredentialAPIKey, Key: key}}
}

// providerCredentialStore answers Read for its one provider and stores nothing else.
type providerCredentialStore struct {
	provider   string
	credential ai.Credential
}

func (store providerCredentialStore) Read(_ context.Context, providerID string) (*ai.Credential, error) {
	if providerID != store.provider {
		return nil, nil
	}
	credential := store.credential
	return &credential, nil
}

func (store providerCredentialStore) List(context.Context) ([]ai.CredentialInfo, error) {
	return []ai.CredentialInfo{{ProviderID: store.provider, Type: store.credential.Type}}, nil
}

func (store providerCredentialStore) Modify(_ context.Context, providerID string, fn func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
	if providerID != store.provider {
		return nil, nil
	}
	credential := store.credential
	return fn(&credential)
}

func (store providerCredentialStore) Delete(context.Context, string) error { return nil }

// emptyCredentialStore stores nothing.
type emptyCredentialStore struct{}

func (emptyCredentialStore) Read(context.Context, string) (*ai.Credential, error) { return nil, nil }

func (emptyCredentialStore) List(context.Context) ([]ai.CredentialInfo, error) { return nil, nil }

func (emptyCredentialStore) Modify(context.Context, string, func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
	return nil, nil
}

func (emptyCredentialStore) Delete(context.Context, string) error { return nil }
