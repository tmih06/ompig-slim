package ai

// Registration of the omp-slim API-key sign-in flows with the OAuth provider registry, the
// per-provider request auth their credentials derive, and the store-side helpers the composite keys
// need.
//
// These are the seven flows of omp_oauth_apikey*.go: Kilo Gateway, Xiaomi MiMo, Alibaba Coding Plan,
// QwenCloud Token Plan, Cloudflare Workers AI, Cloudflare AI Gateway and Mistral Vibe. Each is listed
// in the registry so /login and the auth commands offer it, and each keeps the credential shape its
// provider's request auth reads.

import "errors"

// ompOAuthCredentialEnv is implemented by a sign-in flow whose credential carries provider-scoped
// environment values the request layer needs, such as Cloudflare's account and gateway ids. The
// resolver copies them onto the AuthResult so endpoint placeholders materialize.
type ompOAuthCredentialEnv interface {
	OAuthEnv(creds OAuthCredentials) map[string]string
}

// ompOAuthCredentialEnvFor is the credential-env hook of one of these flows, for the OAuth auth method
// that also resolves the provider's request auth: a flow whose credential carries provider env values
// supplies them (Cloudflare's account and gateway ids), and nil means the credential carries none.
//
// The hook is installed on the OAuth method as `Env`, which resolveStoredOAuth reads into
// AuthResult.Env so endpoint placeholders materialize from a stored login exactly as they do from a
// pasted key.
func ompOAuthCredentialEnvFor(provider OAuthProviderInterface) func(Credential) map[string]string {
	carrier, ok := provider.(ompOAuthCredentialEnv)
	if !ok {
		return nil
	}
	return func(credential Credential) map[string]string {
		return carrier.OAuthEnv(credentialToOAuth(credential))
	}
}

// ompCompositeKeyEnv decodes a Cloudflare composite api key into the provider env values its endpoint
// placeholders need. A legacy plain token supplies none.
func ompCompositeKeyEnv(key string) map[string]string {
	credential, ok := parseCloudflareCredential(key)
	if !ok {
		return nil
	}
	env := map[string]string{}
	if credential.AccountID != "" {
		env[cloudflareAccountID] = credential.AccountID
	}
	if credential.GatewayID != "" {
		env[cloudflareGatewayID] = credential.GatewayID
	}
	if len(env) == 0 {
		return nil
	}
	return env
}

// OAuthEnv reports the account and gateway ids a Workers AI credential carries.
func (cloudflareWorkersAIOAuthProvider) OAuthEnv(creds OAuthCredentials) map[string]string {
	return ompCompositeKeyEnv(creds.Access)
}

// OAuthEnv reports the account and gateway ids an AI Gateway credential carries.
func (cloudflareAIGatewayOAuthProvider) OAuthEnv(creds OAuthCredentials) map[string]string {
	return ompCompositeKeyEnv(creds.Access)
}

// ompOAuthToAuth is the request auth of the omp-slim sign-in flows. It runs before the stock switch so
// a composite credential is decoded once, here, rather than being sent as bearer material.
//
// A Cloudflare credential yields exactly what its API-key method yields, so an OAuth login and a pasted
// JSON key behave identically: Workers AI sends the token as the bearer, the AI Gateway sends it as
// `cf-aig-authorization` with Authorization and x-api-key cleared.
func ompOAuthToAuth(providerID string, credential Credential) (ModelAuth, bool, error) {
	switch providerID {
	case "cloudflare-workers-ai":
		parsed, ok := parseCloudflareCredential(credential.Access)
		if !ok {
			return ModelAuth{APIKey: credential.Access}, true, nil
		}
		return ModelAuth{APIKey: parsed.Token}, true, nil
	case "cloudflare-ai-gateway":
		parsed, ok := parseCloudflareCredential(credential.Access)
		if !ok {
			return ModelAuth{Headers: ProviderHeaders{
				"cf-aig-authorization": new("Bearer " + credential.Access),
				"Authorization":        nil,
				"x-api-key":            nil,
			}}, true, nil
		}
		return ModelAuth{Headers: ProviderHeaders{
			"cf-aig-authorization": new("Bearer " + parsed.Token),
			"Authorization":        nil,
			"x-api-key":            nil,
		}}, true, nil
	case "alibaba-coding-plan":
		// The login remembers the endpoint the key belongs to; a China or custom Coding Plan key only
		// authenticates against it.
		return ModelAuth{APIKey: credential.Access, BaseURL: credential.EnterpriseDomain}, true, nil
	case "alibaba-token-plan":
		// The stored key is the serialized Token Plan credential; the API-key method decodes it.
		parsed, ok := parseAlibabaTokenPlanCredential(credential.Access)
		if !ok {
			return ModelAuth{}, true, errors.New("Invalid QwenCloud Token Plan credential")
		}
		return ModelAuth{APIKey: parsed.Token, BaseURL: parsed.BaseURL}, true, nil
	}
	return ModelAuth{}, false, nil
}
