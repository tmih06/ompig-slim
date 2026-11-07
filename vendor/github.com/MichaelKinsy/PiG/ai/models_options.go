package ai

// Ports packages/ai/src/models.ts (CreateModelsOptions).

// CreateModelsOptions supplies application-owned stores and auth context to model collections.
type CreateModelsOptions struct {
	Credentials CredentialStore
	// AccountRouter routes requests across a provider's stored accounts. Nil builds one over
	// Credentials, so an app that shares a store shares its affinity by passing the same router.
	AccountRouter *AccountRouter
	ModelsStore   ModelsStore
	AuthContext   *AuthContext
	// Identity is the default identity of every request; a request's Identity overrides it field by field.
	Identity *ClientIdentity
}
