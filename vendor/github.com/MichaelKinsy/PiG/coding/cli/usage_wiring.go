package cli

// Wires the quota service behind the /usage board. The service reads the
// session's own credential store, resolves scoped auth through the same
// request-auth runtime model requests use, and is rebuilt whenever the runtime
// replaces the Session, so it never keeps an old store's credentials.

import (
	"context"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
)

// usageServiceOptions assembles the quota service for one session build.
//
// credentials is the session's credential store: the multi-account store when
// it supports accounts, else the single-credential view. auth resolves scoped
// request auth (the account key travels in overrides) so a near-expiry OAuth
// token is refreshed through the normal path, never mutated here.
func usageServiceOptions(
	credentials ai.CredentialStore,
	auth func(context.Context, string, ai.AuthResolutionOverrides) (*ai.AuthResult, error),
	baseURLFor func(string) string,
) codingagent.UsageServiceOptions {
	options := codingagent.UsageServiceOptions{
		Credentials: credentials,
		GetAuth:     auth,
		BaseURLFor:  baseURLFor,
	}
	if accounts, ok := credentials.(ai.AccountCredentialStore); ok {
		options.Accounts = accounts
	}
	return options
}

// newSessionUsageService builds the quota service of one session build.
func newSessionUsageService(build *cliBuild, auth *codingagent.RequestAuthRuntime) *codingagent.UsageService {
	if build == nil || build.Services == nil {
		return nil
	}
	credentials := build.Services.Credentials()
	if credentials == nil {
		return nil
	}
	if auth == nil {
		// A build without a composed request-auth runtime still needs real
		// provider auth: compose one from the same store, and only then fall
		// back to the built-in provider definitions.
		if composed, err := codingagent.NewRequestAuthRuntime(context.Background(), codingagent.RequestAuthRuntimeOptions{
			Credentials: credentials,
			AgentDir:    build.Services.AgentDir(),
		}); err == nil {
			auth = composed
		}
	}
	return codingagent.NewUsageService(usageServiceOptions(
		credentials,
		requestAuthGetAuth(auth, credentials),
		sessionProviderBaseURL(build),
	))
}

// requestAuthGetAuth adapts the request-auth runtime to the quota service's
// scoped resolver. When no runtime exists, resolution uses the provider's
// built-in auth definition; a provider with no definition reports an error
// rather than resolving as if it had no credential.
func requestAuthGetAuth(auth *codingagent.RequestAuthRuntime, credentials ai.CredentialStore) func(context.Context, string, ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
	return func(ctx context.Context, providerID string, overrides ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
		if auth != nil {
			return auth.GetAuth(ctx, providerID, overrides)
		}
		providerAuth, err := ai.BuiltinProviderAuth(providerID)
		if err != nil {
			return nil, err
		}
		return ai.ResolveProviderAuth(ctx, providerID, providerAuth, credentials, ai.DefaultProviderAuthContext(), overrides)
	}
}

// sessionProviderBaseURL reports a provider's configured base URL from the
// session's model registry, so usage probes honor a configured proxy.
func sessionProviderBaseURL(build *cliBuild) func(string) string {
	if build == nil || build.Services == nil {
		return nil
	}
	registry := build.Services.Registry()
	return func(providerID string) string {
		if registry == nil {
			return ""
		}
		provider := registry.GetProvider(providerID)
		if provider == nil {
			return ""
		}
		return provider.BaseURL
	}
}
