package codingagent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestGeminiDecisionSurvivesGoogleProviderOverrides(t *testing.T) {
	dir := t.TempDir()
	models := `{"providers":{"google":{"baseUrl":"http://127.0.0.1:1/v1beta","apiKey":"fixture-only"}}}`
	if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(models), 0o600); err != nil {
		t.Fatal(err)
	}

	registry := NewModelRegistry(dir)
	all := registry.GetProviderAllModelData("google")
	for _, model := range all {
		classifier, ok := model.(*ai.ClassifierModel)
		if !ok || classifier.ID != "gemini-decision-1" {
			continue
		}
		if classifier.Provider != "google" || classifier.API != ai.ClassifierAPIGeminiDecisions || classifier.BaseURL != "http://127.0.0.1:1/v1beta" {
			t.Fatalf("Gemini Decision 1 after provider override = %+v", classifier)
		}
		return
	}
	t.Fatal("Gemini Decision 1 missing from the Google classifier registry after provider overrides")
}
