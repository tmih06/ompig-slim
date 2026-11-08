package ai

import "context"

// codexJSONUsageProvider reuses Codex's probes and header parser, but keeps report
// ownership in the imported provider's namespace for quota-aware account routing.
func codexJSONUsageProvider() *UsageProvider {
	provider := *openAICodexUsageProvider()
	provider.ID = codexJSONProviderID
	provider.Supports = func(params UsageFetchParams) bool {
		return params.Provider == codexJSONProviderID && params.Credential.Type == CredentialOAuth
	}
	provider.FetchUsage = func(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
		params.Provider = "openai-codex"
		report, err := fetchOpenAICodexUsage(ctx, params, fctx)
		return codexJSONUsageReport(report), err
	}
	provider.ParseRateLimitHeaders = func(headers map[string]string, nowMs int64, status *int) *UsageReport {
		return codexJSONUsageReport(parseCodexRateLimitHeaders(headers, nowMs, status))
	}
	return &provider
}

// codexJSONUsageReport relabels a fresh Codex report and its limit scopes in place;
// these reports are newly allocated by the probe/parser and have no other owner.
func codexJSONUsageReport(report *UsageReport) *UsageReport {
	if report == nil {
		return nil
	}
	report.Provider = codexJSONProviderID
	for i := range report.Limits {
		report.Limits[i].Scope.Provider = codexJSONProviderID
	}
	return report
}
