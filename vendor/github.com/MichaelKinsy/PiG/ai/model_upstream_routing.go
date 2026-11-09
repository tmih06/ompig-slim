package ai

import (
	"fmt"
	"maps"
	"strings"
)

// ModelUpstreamRoute returns an explicitly selected OpenRouter/Vercel slug.
// Catalog defaults (even a single provider.only) never become selector suffixes.
func ModelUpstreamRoute(model *Model) string {
	if model == nil {
		return ""
	}
	return model.selectedUpstream
}

// ModelSelector formats physical identity plus an explicit aggregator upstream.
// Literal @-suffixed IDs are left untouched because they have no routing compat.
func ModelSelector(model *Model) string {
	if model == nil {
		return ""
	}
	value := model.ProviderID() + "/" + model.ID
	if upstream := ModelUpstreamRoute(model); upstream != "" {
		value += "@" + upstream
	}
	return value
}

// WithModelUpstreamRoute clones only the metadata it changes. The catalog's model,
// backend and other immutable compat facts stay shared and unmodified. Unsupported
// direct providers reject routing instead of silently ignoring an explicit route.
func WithModelUpstreamRoute(model *Model, upstream string) (*Model, error) {
	if model == nil || upstream == "" {
		return model, nil
	}
	id := model.ProviderID()
	base := strings.ToLower(model.ProviderMeta.BaseURL)
	vercel := id == "vercel-ai-gateway" || strings.Contains(base, "ai-gateway.vercel.sh")
	openrouter := id == "openrouter" || strings.Contains(base, "openrouter.ai")
	if !vercel && !openrouter {
		return nil, fmt.Errorf("provider %s does not support @upstream routing", id)
	}
	out := new(*model)
	out.selectedUpstream = upstream
	compat := ModelCompat{}
	if model.ProviderMeta.Compat != nil {
		compat = *model.ProviderMeta.Compat
	}
	if vercel {
		routing := VercelGatewayRouting{}
		if compat.VercelGatewayRouting != nil {
			routing = *compat.VercelGatewayRouting
		}
		routing.Only = []string{upstream}
		compat.VercelGatewayRouting = &routing
	} else {
		routing := maps.Clone(compat.OpenRouterRouting)
		if routing == nil {
			routing = OpenRouterRouting{}
		}
		routing["only"] = []string{upstream}
		compat.OpenRouterRouting = routing
	}
	out.ProviderMeta.Compat = &compat
	return out, nil
}
