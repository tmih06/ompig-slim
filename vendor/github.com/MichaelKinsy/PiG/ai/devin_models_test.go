package ai

// Devin model-discovery tests: GetCliModelConfigs normalization and the
// credential-scoped roster fetch against an httptest fake.

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Fixture encoders ---------------------------------------------------------

func devinTestFeatures(w *devinPbWriter, images, tools, thinking, parallel bool) {
	w.appendBool(11, images)
	w.appendBool(12, tools)
	w.appendBool(15, thinking)
	w.appendBool(21, parallel)
}

func devinTestModelInfo(display int, harness ...string) []byte {
	w := &devinPbWriter{}
	w.appendVarintField(22, uint64(display))
	features := &devinPbWriter{}
	devinTestFeatures(features, true, true, true, true)
	w.appendMessage(6, features.buffer)
	for _, uid := range harness {
		w.appendString(20, uid)
	}
	return w.buffer
}

func devinTestConfig(uid, label string, mutate func(w *devinPbWriter)) []byte {
	w := &devinPbWriter{}
	w.appendString(1, label)
	w.appendString(22, uid)
	if mutate != nil {
		mutate(w)
	}
	return w.buffer
}

func devinTestDimension(label string, value float32, denominator string, kind int) []byte {
	w := &devinPbWriter{}
	w.appendString(1, label)
	w.appendTag(2, 5)
	w.buffer = binary.LittleEndian.AppendUint32(w.buffer, math.Float32bits(value))
	w.appendString(3, denominator)
	w.appendVarintField(6, uint64(kind))
	return w.buffer
}

func devinTestConfigsResponse(configs ...[]byte) []byte {
	w := &devinPbWriter{}
	for _, config := range configs {
		w.appendMessage(1, config)
	}
	return w.buffer
}

func devinTestDefaultOverride(uid string) []byte {
	w := &devinPbWriter{}
	w.appendString(3, uid)
	return w.buffer
}

// --- Tests ---------------------------------------------------------------------

func TestDevinNormalizeModelConfigs(t *testing.T) {
	leadConfig := devinTestConfig("swe-lead", "SWE Lead", func(w *devinPbWriter) {
		w.appendMessage(23, devinTestModelInfo(8))
		w.appendVarintField(18, 256000)
		info := &devinPbWriter{}
		info.appendVarintField(4, 262144)
		info.appendVarintField(13, 32768)
		// modelInfo already appended above lacks limits; rebuild properly below.
		_ = info
	})
	_ = leadConfig

	// Build configs with full modelInfo (limits + features + display).
	fullInfo := func(display int, ctxTokens, maxOut int32, thinking bool) []byte {
		w := &devinPbWriter{}
		w.appendVarintField(4, uint64(ctxTokens))
		w.appendVarintField(13, uint64(maxOut))
		w.appendVarintField(22, uint64(display))
		features := &devinPbWriter{}
		devinTestFeatures(features, true, true, thinking, true)
		w.appendMessage(6, features.buffer)
		return w.buffer
	}

	lead := devinTestConfig("swe-lead", "SWE Lead", func(w *devinPbWriter) {
		w.appendMessage(23, fullInfo(8, 262144, 32768, true))
		w.appendMessage(32, devinTestDimension("input", 3, "1M tokens", 1))
		w.appendMessage(32, devinTestDimension("cached input", 0.3, "1M tokens", 2))
		w.appendMessage(32, devinTestDimension("output", 15, "1M tokens", 1))
		w.appendMessage(32, devinTestDimension("sidekick", 99, "1M tokens", 1))
		w.appendMessage(32, devinTestDimension("input", 100, "1M tokens", 1)) // after marker: ignored
	})
	router := devinTestConfig("adaptive", "Adaptive", func(w *devinPbWriter) {
		// display MODEL_ROUTER(3), no harness uids, no features.
		info := &devinPbWriter{}
		info.appendVarintField(22, 3)
		w.appendMessage(23, info.buffer)
	})
	fusion := devinTestConfig("fusion-swe-lead-sidekick-swe-2", "Fusion", func(w *devinPbWriter) {
		w.appendMessage(23, fullInfo(8, 100000, 8000, false))
	})
	fusionDead := devinTestConfig("fusion-gone-sidekick-x", "Fusion Dead", func(w *devinPbWriter) {
		w.appendMessage(23, fullInfo(8, 100000, 8000, false))
	})
	disabled := devinTestConfig("swe-old", "Old", func(w *devinPbWriter) {
		w.appendVarintField(4, 1)
		w.appendMessage(23, fullInfo(8, 100000, 8000, false))
	})
	internal := devinTestConfig("quick", "Quick", func(w *devinPbWriter) {
		w.appendMessage(23, fullInfo(6, 100000, 8000, false))
	})
	quickReview := devinTestConfig("qr", "QR", func(w *devinPbWriter) {
		w.appendMessage(23, fullInfo(4, 100000, 8000, false))
	})
	dup := devinTestConfig("swe-lead", "SWE Lead Dup", func(w *devinPbWriter) {
		w.appendMessage(23, fullInfo(8, 1, 1, false))
	})
	blind := devinTestConfig("swe-1-6", "SWE 1.6", func(w *devinPbWriter) {
		// config-level supportsImages but uid is image-blind.
		w.appendVarintField(5, 1)
		w.appendMessage(23, fullInfo(8, 200000, 64000, true))
	})

	response := &devinCliModelConfigsResponse{}
	raw := devinTestConfigsResponse(lead, router, fusion, fusionDead, disabled, internal, quickReview, dup, blind)
	response = decodeDevinCliModelConfigsResponse(raw)
	if response == nil {
		t.Fatal("response decode failed")
	}
	// Attach default override through the wire too.
	override := &devinPbWriter{}
	override.appendMessage(3, devinTestDefaultOverride("swe-lead"))
	combined := &devinPbWriter{}
	combined.buffer = append(combined.buffer, raw...)
	combined.buffer = append(combined.buffer, override.buffer...)
	response = decodeDevinCliModelConfigsResponse(combined.buffer)

	models := normalizeDevinModelConfigs(response, "https://example.test")
	byID := map[string]*Model{}
	for _, model := range models {
		byID[model.ID] = model
	}
	for _, gone := range []string{"swe-old", "quick", "qr", "fusion-gone-sidekick-x", "adaptive"} {
		_ = gone
	}
	if _, ok := byID["swe-old"]; ok {
		t.Error("disabled model listed")
	}
	if _, ok := byID["quick"]; ok {
		t.Error("internal display 6 listed")
	}
	if _, ok := byID["qr"]; ok {
		t.Error("QUICK_REVIEW display listed")
	}
	if _, ok := byID["fusion-gone-sidekick-x"]; ok {
		t.Error("fusion with dead lead listed")
	}
	leadModel := byID["swe-lead"]
	if leadModel == nil {
		t.Fatal("lead model missing")
	}
	if leadModel.DisplayName != "SWE Lead" {
		t.Fatalf("name %q", leadModel.DisplayName)
	}
	if !leadModel.ProviderMeta.Reasoning {
		t.Error("lead should be reasoning")
	}
	if leadModel.Capabilities.ContextWindow != 262144 || leadModel.Capabilities.MaxOutputTokens != 32768 {
		t.Fatalf("limits %#v", leadModel.Capabilities)
	}
	if leadModel.Capabilities.InputCostPer1M != 3 || leadModel.Capabilities.OutputCostPer1M != 15 || leadModel.Capabilities.CacheReadCostPer1M != 0.3 {
		t.Fatalf("cost %#v", leadModel.Capabilities)
	}
	if leadModel.ProviderMeta.Compat.Extra["isProviderDefault"] == nil {
		t.Error("default override not marked")
	}
	var parallel bool
	if raw, ok := leadModel.ProviderMeta.Compat.Extra["supportsParallelToolCalls"]; ok {
		_ = json.Unmarshal(raw, &parallel)
	}
	if !parallel {
		t.Error("parallel tool calls flag missing")
	}
	routerModel := byID["adaptive"]
	if routerModel == nil {
		t.Fatal("router model missing")
	}
	if _, ok := routerModel.ProviderMeta.Compat.Extra["modelRouter"]; !ok {
		t.Error("router compat.modelRouter missing")
	}
	if !routerModel.Capabilities.SupportsToolUse {
		t.Error("router without features should default to tool support")
	}
	fusionModel := byID["fusion-swe-lead-sidekick-swe-2"]
	if fusionModel == nil {
		t.Fatal("fusion model missing")
	}
	var requestModel string
	if raw, ok := fusionModel.ProviderMeta.Compat.Extra["requestModelId"]; ok {
		_ = json.Unmarshal(raw, &requestModel)
	}
	if requestModel != "swe-lead" {
		t.Fatalf("fusion requestModelId %q", requestModel)
	}
	if fusionModel.Capabilities.ContextWindow != 262144 {
		t.Fatalf("fusion should inherit lead limits, got %d", fusionModel.Capabilities.ContextWindow)
	}
	blindModel := byID["swe-1-6"]
	if blindModel == nil {
		t.Fatal("blind model missing")
	}
	for _, modality := range blindModel.Input {
		if modality == ModelInputImage {
			t.Error("swe-1-6 must not advertise images")
		}
	}
	// Sorted by id.
	for i := 1; i < len(models); i++ {
		if models[i-1].ID > models[i].ID {
			t.Fatalf("roster not sorted: %q > %q", models[i-1].ID, models[i].ID)
		}
	}
}

func TestDevinFetchModelRoster(t *testing.T) {
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/proto")
		_, _ = w.Write(devinTestConfigsResponse(devinTestConfig("m1", "Model One", func(w *devinPbWriter) {
			w.appendMessage(23, devinTestModelInfo(8))
		})))
	}))
	defer server.Close()

	credential := &Credential{Type: CredentialOAuth, Access: devinTestJWT(9999999999)}
	models, err := fetchDevinModelRoster(context.Background(), server.URL, credential)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotPath != devinCliModelConfigsPath {
		t.Fatalf("path %q", gotPath)
	}
	// Discovery metadata announces the chisel dev identity.
	if meta := devinTestReadMetadata(devinTestReadChatRequestMetadata(t, gotBody)); meta.ideName != "chisel" {
		t.Fatalf("discovery ideName %q", meta.ideName)
	}
	if len(models) != 1 || models[0].ID != "m1" {
		t.Fatalf("models %#v", models)
	}
	if models[0].ProviderMeta.API != APIDevinAgent || models[0].ProviderMeta.ProviderID != "devin" {
		t.Fatalf("provider meta %#v", models[0].ProviderMeta)
	}
}

// devinTestReadChatRequestMetadata unwraps field 1 (metadata) of a request body.
func devinTestReadChatRequestMetadata(t *testing.T, data []byte) []byte {
	t.Helper()
	reader := &devinPbReader{data: data}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			break
		}
		if wire == 2 {
			payload, ok := reader.readBytes()
			if !ok {
				break
			}
			if field == 1 {
				return payload
			}
			continue
		}
		if !reader.skip(wire) {
			break
		}
	}
	t.Fatal("no metadata field")
	return nil
}

func TestDevinFetchModelRosterEmptyKeepsSeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/proto")
		_, _ = w.Write(devinTestConfigsResponse())
	}))
	defer server.Close()
	credential := &Credential{Type: CredentialOAuth, Access: "tok"}
	if _, err := fetchDevinModelRoster(context.Background(), server.URL, credential); err == nil {
		t.Fatal("expected empty-roster error")
	}
}

func TestDevinModelScopeStoreKeyStability(t *testing.T) {
	provider := &ModelsProvider{ID: devinProviderName, BaseURL: "https://server.codeium.com"}
	WithDevinModelDiscovery(provider)

	// A credential with stable identity fields persists its roster snapshot.
	stable := provider.ModelCatalog.Scope(provider.BaseURL, &Credential{
		Type:      CredentialOAuth,
		AccountID: "acct-1",
		Access:    "session-token",
	})
	if stable.ID == "" || stable.StoreKey == "" {
		t.Fatalf("stable scope %+v", stable)
	}

	// An ephemeral access-token scope gets a runtime scope id but is never
	// persisted under a store key.
	ephemeral := provider.ModelCatalog.Scope(provider.BaseURL, &Credential{
		Type:   CredentialOAuth,
		Access: "ephemeral-token",
	})
	if ephemeral.ID == "" {
		t.Fatal("ephemeral scope has no id")
	}
	if ephemeral.StoreKey != "" {
		t.Fatalf("ephemeral scope persisted: %+v", ephemeral)
	}
	if strings.Contains(ephemeral.ID, "ephemeral-token") {
		t.Fatalf("scope id leaks token: %q", ephemeral.ID)
	}
	if stable.ID == ephemeral.ID {
		t.Fatal("stable and ephemeral credentials share a scope")
	}
}
