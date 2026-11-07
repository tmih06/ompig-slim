package coding

// Ports packages/coding-agent/src/core/model-runtime.ts.

import (
	"context"

	"github.com/MichaelKinsy/PiG/ai"
)

// registryOwnsModelBackend distinguishes lazy registry bindings from caller-owned callbacks. Metadata alone does not transfer callback or authentication ownership.
func registryOwnsModelBackend(provider ai.Provider) bool {
	switch provider.(type) {
	case nil, *catalogModelBackend, *nativeModelBackend, *nativeModelProvider, *requestOptionsProvider:
		return true
	default:
		return false
	}
}

func (runtime *ModelRuntime) bindCatalogModel(model *ai.Model) *ai.Model {
	if runtime.services.Registry().GetProvider(model.ProviderMeta.ProviderID) != nil {
		return runtime.bindNativeModel(model)
	}
	bound := new(*model)
	bound.Provider = &catalogModelBackend{runtime: runtime, model: model}
	return bound
}

type catalogModelBackend struct {
	runtime *ModelRuntime
	model   *ai.Model
}

func (provider *catalogModelBackend) ID() string { return provider.model.ProviderMeta.ProviderID }
func (*catalogModelBackend) Close() error        { return nil }
func (provider *catalogModelBackend) Stream(ctx context.Context, transcript ai.TranscriptContext, options ai.StreamOptions) (*ai.AssistantMessageEventStream, error) {
	bound := new(*provider.model)
	bound.Provider = provider
	// A direct provider call — compaction's completeSimple among them — shares the request's
	// account routing, so a rejected account rotates here too.
	router := provider.runtime.accountRouter()
	return router.StreamRoutes(ctx, bound, ai.AccountRouteRequest{SessionID: options.SessionID, APIKeySet: options.APIKey != "" || options.APIKeySet}, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, forceOAuthRefresh bool) (*ai.AssistantMessageEventStream, error) {
		attemptCtx = ai.WithAccountSelection(attemptCtx, selection, forceOAuthRefresh)
		attemptOptions := options
		attemptOptions.OnResponse = ai.ObserveOnResponse(attemptOptions.OnResponse, observe)
		_, backend, prepared, err := provider.runtime.prepare(attemptCtx, bound, attemptOptions)
		if err != nil {
			return nil, err
		}
		return backend.Stream(attemptCtx, transcript, prepared)
	}), nil
}
