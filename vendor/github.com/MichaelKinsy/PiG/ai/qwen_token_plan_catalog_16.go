package ai

import (
	"slices"
	"strings"
)

const (
	qwenTokenPlanUSBaseURL              = "https://token-plan.maas.qwencloudapi.com/compatible-mode/v1"
	qwenTokenPlanCNBaseURL              = "https://token-plan.maas.qianwenaiapi.com/compatible-mode/v1"
	qwenTokenPlanIndividualCNProviderID = "qwen-token-plan-individual-cn"
)

func init() {
	for i := range GeneratedModels {
		model := &GeneratedModels[i]
		if strings.HasPrefix(model.Provider, "qwen-token-plan") {
			baseURL := qwenTokenPlanUSBaseURL
			if strings.HasSuffix(model.Provider, "-cn") {
				baseURL = qwenTokenPlanCNBaseURL
			}
			model.Record = strings.Replace(model.Record, `"baseUrl":"`+model.BaseURL+`"`, `"baseUrl":"`+baseURL+`"`, 1)
			model.BaseURL = baseURL
		}
	}

	for _, id := range []string{"deepseek-v4.1-flash", "deepseek-v4.1-flash-1017", "glm-5.3"} {
		if model := findQwenCatalogModel("qwen-token-plan", id); model != nil {
			GeneratedModels = append(GeneratedModels, cloneQwenCatalogModel(model, "qwen-token-plan-individual", qwenTokenPlanUSBaseURL))
		}
	}
	if model := findQwenCatalogModel("qwen-token-plan-individual", "qwen3.8-max"); model != nil {
		preview := cloneQwenCatalogModel(model, "qwen-token-plan-individual", qwenTokenPlanUSBaseURL)
		preview.ID = "qwen4-max-preview"
		preview.DisplayName = "Qwen4 Max Preview"
		preview.ContextWindow = 1_000_000
		preview.MaxOutputTokens = 393_216
		preview.ThinkingLevelMap = ThinkingLevelMap{
			ThinkingOff: nil, ThinkingMinimal: nil, ThinkingLow: new("low"), ThinkingMedium: new("medium"),
			ThinkingHigh: nil, ThinkingXHigh: new("xhigh"), ThinkingMax: nil,
		}
		preview.Record = strings.ReplaceAll(preview.Record, `"qwen3.8-max"`, `"qwen4-max-preview"`)
		preview.Record = strings.ReplaceAll(preview.Record, `"Qwen3.8 Max"`, `"Qwen4 Max Preview"`)
		preview.Record = strings.Replace(preview.Record, `"maxTokens":131072`, `"maxTokens":393216`, 1)
		GeneratedModels = append(GeneratedModels, preview)
	}

	individualModels := make([]GeneratedModel, 0)
	for i := range GeneratedModels {
		if GeneratedModels[i].Provider == "qwen-token-plan-individual" {
			individualModels = append(individualModels, GeneratedModels[i])
		}
	}
	for i := range individualModels {
		GeneratedModels = append(GeneratedModels, cloneQwenCatalogModel(&individualModels[i], qwenTokenPlanIndividualCNProviderID, qwenTokenPlanCNBaseURL))
	}

	if index := slices.Index(GeneratedProviders, "qwen-token-plan-individual"); index >= 0 && !slices.Contains(GeneratedProviders, qwenTokenPlanIndividualCNProviderID) {
		GeneratedProviders = slices.Insert(GeneratedProviders, index+1, qwenTokenPlanIndividualCNProviderID)
	}
	builtinProviderNames()[qwenTokenPlanIndividualCNProviderID] = "Qwen Token Plan Individual CN"
	builtinProviderBaseURLs()[qwenTokenPlanIndividualCNProviderID] = qwenTokenPlanCNBaseURL
	// The auth table can be snapshotted before this init runs (google_cloud_code_registration.go fires it first),
	// so the method is registered on the table itself, as that registration does.
	builtinAPIKeyAuths()[qwenTokenPlanIndividualCNProviderID] = EnvAPIKeyAuth(
		"Qwen Token Plan Individual CN API key", getAPIKeyEnvVars(qwenTokenPlanIndividualCNProviderID)...)
	builtinProviderConstructors[qwenTokenPlanIndividualCNProviderID] = QwenTokenPlanIndividualCNProvider
}

func findQwenCatalogModel(provider, id string) *GeneratedModel {
	for i := range GeneratedModels {
		if GeneratedModels[i].Provider == provider && GeneratedModels[i].ID == id {
			return &GeneratedModels[i]
		}
	}
	return nil
}

func cloneQwenCatalogModel(source *GeneratedModel, provider, baseURL string) GeneratedModel {
	model := *source
	model.Record = strings.Replace(model.Record, `"provider":"`+source.Provider+`"`, `"provider":"`+provider+`"`, 1)
	model.Record = strings.Replace(model.Record, `"baseUrl":"`+source.BaseURL+`"`, `"baseUrl":"`+baseURL+`"`, 1)
	model.Provider = provider
	model.BaseURL = baseURL
	return model
}

// QwenTokenPlanIndividualCNProvider is the built-in Qwen Token Plan Individual CN provider.
func QwenTokenPlanIndividualCNProvider() *ModelsProvider {
	return builtinProvider(qwenTokenPlanIndividualCNProviderID)
}
