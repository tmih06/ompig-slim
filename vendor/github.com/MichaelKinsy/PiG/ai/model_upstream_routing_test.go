package ai

import "testing"

// TestExplicitUpstreamDoesNotRewriteCatalog verifies selector routing is private
// per-selection state, never inferred from a literal ID or shared catalog compat.
func TestExplicitUpstreamDoesNotRewriteCatalog(t *testing.T) {
	original := &Model{ID: "literal@default", ProviderMeta: ProviderMetadata{ProviderID: "openrouter", Compat: &ModelCompat{OpenRouterRouting: OpenRouterRouting{"only": []string{"catalog-default"}}}}}
	if got := ModelSelector(original); got != "openrouter/literal@default" {
		t.Fatal(got)
	}
	routed, err := WithModelUpstreamRoute(original, "fireworks")
	if err != nil {
		t.Fatal(err)
	}
	if ModelsAreEqual(original, routed) || ModelSelector(routed) != "openrouter/literal@default@fireworks" {
		t.Fatal("explicit route not distinct")
	}
	if original.ProviderMeta.Compat.OpenRouterRouting["only"].([]string)[0] != "catalog-default" || ModelUpstreamRoute(original) != "" {
		t.Fatal("mutated catalog")
	}
	vercel, err := WithModelUpstreamRoute(&Model{ID: "test", ProviderMeta: ProviderMetadata{ProviderID: "vercel-ai-gateway"}}, "anthropic")
	if err != nil || vercel.ProviderMeta.Compat.VercelGatewayRouting.Only[0] != "anthropic" {
		t.Fatal(vercel, err)
	}
	if _, err := WithModelUpstreamRoute(&Model{ProviderMeta: ProviderMetadata{ProviderID: "openai"}}, "ignored"); err == nil {
		t.Fatal("silently accepted unsupported routing")
	}
}
