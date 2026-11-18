package codingagent

// Wires the remote catalog overlay into the built-in providers of one ModelRegistry.
// upstream: packages/coding-agent/src/core/model-runtime.ts:create (withRemoteCatalog over every built-in provider except radius)

import (
	"context"
	"slices"

	"github.com/MichaelKinsy/PiG/ai"
)

// SetCatalogBaseURL selects the remote catalog endpoint; empty selects DefaultCatalogBaseURL.
// upstream: model-runtime.ts:CreateModelRuntimeOptions.catalogBaseUrl
func (r *ModelRegistry) SetCatalogBaseURL(baseURL string) {
	r.remoteMu.Lock()
	r.catalogBaseURL = baseURL
	r.remoteMu.Unlock()
}

func (r *ModelRegistry) claimInteractiveCatalogRefresh() bool {
	r.remoteMu.Lock()
	defer r.remoteMu.Unlock()
	if r.interactiveCatalogRefreshDone {
		return false
	}
	r.interactiveCatalogRefreshDone = true
	return true
}

func (r *ModelRegistry) remoteOverlay(providerID string) *remoteCatalogOverlay {
	r.remoteMu.Lock()
	defer r.remoteMu.Unlock()
	if r.remoteOverlays == nil {
		r.remoteOverlays = map[string]*remoteCatalogOverlay{}
	}
	overlay := r.remoteOverlays[providerID]
	if overlay == nil {
		overlay = &remoteCatalogOverlay{replaces: providerID == openRouterProviderID}
		r.remoteOverlays[providerID] = overlay
	}
	return overlay
}

// remoteOverlayModels returns the models the remote catalog added to a built-in provider, without creating state for providers that have none.
func (r *ModelRegistry) remoteOverlayModels(providerID string) []ai.AnyModel {
	r.remoteMu.Lock()
	overlay := r.remoteOverlays[providerID]
	r.remoteMu.Unlock()
	if overlay == nil {
		return nil
	}
	return overlay.snapshot()
}

// remoteOverlayReplaces reports whether the provider's overlay is its whole catalog (a personalized one) rather than additions to the bundled catalog.
func (r *ModelRegistry) remoteOverlayReplaces(providerID string) bool {
	r.remoteMu.Lock()
	overlay := r.remoteOverlays[providerID]
	r.remoteMu.Unlock()
	return overlay != nil && overlay.replaces
}

func (r *ModelRegistry) remoteModelsCollection() *ai.Models {
	r.remoteMu.Lock()
	defer r.remoteMu.Unlock()
	return r.remoteCollection
}

func (r *ModelRegistry) codexDiscoveryProvider(providerID string) *ai.ModelsProvider {
	collection := r.remoteModelsCollection()
	if collection == nil {
		return nil
	}
	provider := collection.GetProvider(providerID)
	if provider == nil || provider.ModelCatalog == nil {
		return nil
	}
	return provider
}

// builtinBase is the built-in provider with its remote catalog overlay, or nil for a provider with no built-in model of any type.
func (r *ModelRegistry) builtinBase(id string, fallback ai.ModelsStreamFunction) *ai.ModelsProvider {
	base := r.withPublicCatalog(id, builtinTypedBase(id, fallback))
	if base == nil || id == RadiusProviderID {
		return base
	}
	r.remoteMu.Lock()
	baseURL := r.catalogBaseURL
	r.remoteMu.Unlock()
	// The OMP snapshot owns these bundled identities. The inherited PiG
	// endpoint serves a different catalog and would reintroduce stale rows and
	// synthetic variants on F5. Explicit endpoint overrides remain supported;
	// native provider discovery and public metric refresh keep their own paths.
	if base.ModelCatalog != nil || baseURL == "" && ai.HasOMPCatalog(id) {
		return base
	}
	overlay := r.remoteOverlay(id)
	// OpenRouter narrows the catalog to the models, limits and prices of the user's key.
	// upstream: model-runtime.ts:create (provider.id === "openrouter" selects withPersonalizedCatalog and personalizeOpenRouterCatalog)
	if id == openRouterProviderID {
		return withPersonalizedCatalogOverlay(base, ai.PersonalizeOpenRouterCatalog, baseURL, builtinModelDataGeneratedAt(), overlay)
	}
	return withRemoteCatalogOverlay(base, baseURL, builtinModelDataGeneratedAt(), overlay)
}

// openRouterProviderID is the built-in provider whose catalog is personalized with the user's key (model-runtime.ts:create).
const openRouterProviderID = "openrouter"

// providerBaseURLOverride is the base URL an extension registration or models.json sets for a provider, or "" when neither does; an extension's wins, as in
// resolveGeneratedModelLocked and the dynamic provider resolution.
func (r *ModelRegistry) providerBaseURLOverride(id string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if dynamic, ok := r.dynamic[id]; ok && dynamic.Models == nil && dynamic.BaseURL != "" {
		return dynamic.BaseURL
	}
	if r.config != nil {
		if provider, ok := r.config.Providers[id]; ok {
			return provider.BaseURL
		}
	}
	return ""
}

// remoteCatalogProviderIDs are the built-in providers whose catalogs the overlay refreshes: not Radius, which has its own catalog, and not
// a provider an extension replaced, whose composed base refreshes through the native collection.
func (r *ModelRegistry) remoteCatalogProviderIDs(selected []string) []string {
	var ids []string
	for _, id := range ai.ListProviders() {
		if r.isRadiusProvider(id) || r.GetProvider(id) != nil {
			continue
		}
		if len(selected) > 0 && !slices.Contains(selected, id) {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func (r *ModelRegistry) isRadiusProvider(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.radius[id] != nil || id == RadiusProviderID
}

// refreshRemoteCatalogs restores cached provider snapshots and revalidates them only when allowed.
func (r *ModelRegistry) refreshRemoteCatalogs(ctx context.Context, options ai.ModelsRefreshOptions) ai.ModelsRefreshResult {
	ids := r.remoteCatalogProviderIDs(options.Providers)
	if len(ids) == 0 {
		return ai.ModelsRefreshResult{Errors: map[string]error{}}
	}
	r.remoteMu.Lock()
	if r.remoteCollection == nil {
		r.remoteCollection = ai.CreateModelsWithModelAuth(func(ctx context.Context, model *ai.Model, overrides ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
			return r.ResolveRegistryModelAuth(ctx, model, overrides)
		}, ai.CreateModelsOptions{Credentials: registryCredentials{r}, AccountRouter: r.AccountRouter(), ModelsStore: registryModelsStore{r}})
	}
	collection := r.remoteCollection
	r.remoteMu.Unlock()
	for _, id := range ids {
		override := r.providerBaseURLOverride(id)
		r.remoteMu.Lock()
		registered, applied := collection.GetProvider(id) != nil, r.remoteBaseURLs[id]
		r.remoteMu.Unlock()
		if registered && applied == override {
			continue
		}
		if base := r.builtinBase(id, nil); base != nil {
			if override != "" {
				if base.ModelCatalog != nil {
					base.BaseURL = override
				} else {
					overridden := *base
					overridden.BaseURL = override
					base = &overridden
				}
			}
			collection.SetProvider(base)
			r.remoteMu.Lock()
			if r.remoteBaseURLs == nil {
				r.remoteBaseURLs = map[string]string{}
			}
			r.remoteBaseURLs[id] = override
			r.remoteMu.Unlock()
		}
	}
	options.Providers = ids
	result := ai.ModelsRefreshResult{Aborted: true, Errors: map[string]error{}}
	done := make(chan struct{})
	if !r.StartModelTask(ctx, func(ctx context.Context) {
		defer close(done)
		result = r.refreshRemoteCollection(ctx, collection, ids, options)
	}) {
		return result
	}
	<-done
	return result
}

type codexRefreshAccount struct {
	key     *string
	keySet  bool
	scopeID string
}

func (r *ModelRegistry) refreshRemoteCollection(ctx context.Context, collection *ai.Models, ids []string, options ai.ModelsRefreshOptions) ai.ModelsRefreshResult {
	result := ai.ModelsRefreshResult{Errors: map[string]error{}}
	var ordinary []string
	for _, id := range ids {
		provider := collection.GetProvider(id)
		if provider == nil || provider.ModelCatalog == nil {
			ordinary = append(ordinary, id)
			continue
		}
		targets, multiAccount, err := r.codexRefreshAccounts(ctx, id, provider)
		if err != nil {
			result.Errors[id] = err
			result.ErrorOrder = append(result.ErrorOrder, id)
			continue
		}
		offline := options
		offline.AllowNetwork = new(false)
		if len(targets) == 0 && !multiAccount {
			mergeRefreshResult(&result, collection.Refresh(ctx, codexProviderRefreshOptions(offline, id)))
		} else {
			for _, target := range targets {
				mergeRefreshResult(&result, collection.Refresh(target.context(ctx), codexProviderRefreshOptions(offline, id)))
			}
		}
		if ctx.Err() != nil {
			result.Aborted = true
			return result
		}
		provider.ModelCatalog.RetainScopes(provider.BaseURL, codexRefreshScopeIDs(targets))
		if options.AllowNetwork == nil || *options.AllowNetwork {
			for _, target := range targets {
				mergeRefreshResult(&result, collection.Refresh(target.context(ctx), codexProviderRefreshOptions(options, id)))
				if ctx.Err() != nil {
					result.Aborted = true
					return result
				}
			}
		}
		current, _, err := r.codexRefreshAccounts(ctx, id, provider)
		if err == nil {
			provider.ModelCatalog.RetainScopes(provider.BaseURL, codexRefreshScopeIDs(current))
		}
	}
	if len(ordinary) > 0 && ctx.Err() == nil {
		ordinaryOptions := options
		ordinaryOptions.Providers = ordinary
		mergeRefreshResult(&result, collection.Refresh(ctx, ordinaryOptions))
	}
	result.Aborted = result.Aborted || ctx.Err() != nil
	return result
}

func (r *ModelRegistry) codexRefreshAccounts(ctx context.Context, providerID string, provider *ai.ModelsProvider) ([]codexRefreshAccount, bool, error) {
	if key, ok := r.RuntimeAPIKey(providerID); ok && key != "" {
		return nil, true, nil
	}
	accounts := registryCredentialAccounts{registry: r}
	if store := accounts.store(); store != nil {
		infos, err := accounts.ListAccounts(ctx)
		if err != nil {
			return nil, true, err
		}
		var targets []codexRefreshAccount
		for _, info := range infos {
			if info.ProviderID != providerID {
				continue
			}
			credential, err := (registryCredentials{r}).ReadAccount(ctx, providerID, info.AccountKey)
			if err != nil {
				return nil, true, err
			}
			if credential == nil || credential.Type != ai.CredentialOAuth {
				continue
			}
			scope := provider.ModelCatalog.Scope(provider.BaseURL, credential)
			if scope.ID == "" {
				continue
			}
			var key *string
			if info.AccountKey != nil {
				copy := *info.AccountKey
				key = &copy
			}
			targets = append(targets, codexRefreshAccount{key: key, keySet: true, scopeID: scope.ID})
		}
		return targets, true, nil
	}
	credential, err := (registryCredentials{r}).Read(ctx, providerID)
	if err != nil {
		return nil, false, err
	}
	var scopeID string
	if credential != nil && credential.Type == ai.CredentialOAuth {
		scopeID = provider.ModelCatalog.Scope(provider.BaseURL, credential).ID
	}
	return []codexRefreshAccount{{scopeID: scopeID}}, false, nil
}

func (account codexRefreshAccount) context(ctx context.Context) context.Context {
	if !account.keySet {
		return ctx
	}
	return ai.WithAccountRequest(ctx, ai.AccountRequest{AccountKey: account.key, AccountKeySet: true})
}

func codexRefreshScopeIDs(accounts []codexRefreshAccount) []string {
	scopes := make([]string, 0, len(accounts))
	for _, account := range accounts {
		if account.scopeID != "" {
			scopes = append(scopes, account.scopeID)
		}
	}
	return scopes
}

func codexProviderRefreshOptions(options ai.ModelsRefreshOptions, providerID string) ai.ModelsRefreshOptions {
	options.Providers = []string{providerID}
	return options
}

func mergeRefreshResult(destination *ai.ModelsRefreshResult, source ai.ModelsRefreshResult) {
	destination.Aborted = destination.Aborted || source.Aborted
	if destination.Errors == nil {
		destination.Errors = map[string]error{}
	}
	for _, id := range source.ErrorOrder {
		if err := source.Errors[id]; err != nil {
			if _, exists := destination.Errors[id]; !exists {
				destination.ErrorOrder = append(destination.ErrorOrder, id)
				destination.Errors[id] = err
			}
		}
	}
	for id, err := range source.Errors {
		if err != nil {
			if _, exists := destination.Errors[id]; !exists {
				destination.ErrorOrder = append(destination.ErrorOrder, id)
				destination.Errors[id] = err
			}
		}
	}
}
