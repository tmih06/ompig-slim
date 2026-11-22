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
	"time"
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

// devinInfo builds a minimal modelInfo with limits and the STANDARD display
// slot so normalize keeps the config.
func devinTestFullInfo(ctxTokens, maxOut int32) []byte {
	w := &devinPbWriter{}
	w.appendVarintField(4, uint64(ctxTokens))
	w.appendVarintField(13, uint64(maxOut))
	w.appendVarintField(22, 8)
	features := &devinPbWriter{}
	devinTestFeatures(features, true, true, true, true)
	w.appendMessage(6, features.buffer)
	return w.buffer
}

func devinNormalizeForTest(t *testing.T, configs ...[]byte) map[string]*Model {
	t.Helper()
	response := decodeDevinCliModelConfigsResponse(devinTestConfigsResponse(configs...))
	if response == nil {
		t.Fatal("response decode failed")
	}
	byID := map[string]*Model{}
	for _, model := range normalizeDevinModelConfigs(response, "https://example.test") {
		if _, dup := byID[model.ID]; dup {
			t.Fatalf("duplicate model id %q", model.ID)
		}
		byID[model.ID] = model
	}
	return byID
}

// TestDevinCostFallbackPolicy checks the KDL cost-fallback contract: wire
// prices stay authoritative, unpriced uids get the documented card.
func TestDevinCostFallbackPolicy(t *testing.T) {
	unpriced := devinTestConfig("swe-2-max", "SWE-2 Max", func(w *devinPbWriter) {
		w.appendMessage(23, devinTestFullInfo(262144, 32768))
	})
	priced := devinTestConfig("swe-2-medium", "SWE-2 Medium", func(w *devinPbWriter) {
		w.appendMessage(23, devinTestFullInfo(262144, 32768))
		w.appendMessage(32, devinTestDimension("input", 9, "1M tokens", 1))
		w.appendMessage(32, devinTestDimension("output", 9, "1M tokens", 1))
	})
	byID := devinNormalizeForTest(t, unpriced, priced)
	if got := byID["swe-2-max"].Capabilities; got.InputCostPer1M != 0.5 || got.OutputCostPer1M != 2.5 || got.CacheReadCostPer1M != 0.05 {
		t.Fatalf("swe-2-max fallback %#v", got)
	}
	if got := byID["swe-2-medium"].Capabilities; got.InputCostPer1M != 9 || got.OutputCostPer1M != 9 {
		t.Fatalf("wire price must win over fallback, got %#v", got)
	}
}

// TestDevinDatedCostCards checks promo-to-list card selection and the dated
// cost-patch overrides.
func TestDevinDatedCostCards(t *testing.T) {
	promo := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	list := time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)
	card, ok := devinEffectiveCostCard("swe-2", promo)
	if !ok || card.Input != 0.5 || card.Output != 2.5 {
		t.Fatalf("promo card %#v", card)
	}
	card, ok = devinEffectiveCostCard("swe-2", list)
	if !ok || card.Input != 3 || card.Output != 15 || card.CacheRead != 0.3 {
		t.Fatalf("list card %#v", card)
	}
	if _, ok := devinEffectiveCostCard("unlisted-uid", promo); ok {
		t.Fatal("unlisted uid must not price")
	}
	if _, ok := devinCostPatchCard("gemini-4-carbon", promo); ok {
		t.Fatal("carbon list price before its effective date")
	}
	card, ok = devinCostPatchCard("gemini-4-carbon-high", list)
	if !ok || card.Input != 4 || card.Output != 20 {
		t.Fatalf("carbon patch %#v", card)
	}
	if _, ok := devinCostPatchCard("gemini-3-8-flash", list); !ok {
		t.Fatal("gemini-3.8-flash misses the class list card")
	}
	card, ok = devinCostPatchCard("gemini-3-7-flash-high-lightning", list)
	if !ok || card.Input != 1.5 {
		t.Fatalf("flash lane patch %#v", card)
	}
}

// TestDevinCacheWriteRate checks the 1.25x cache-write multiplier lineage rule.
func TestDevinCacheWriteRate(t *testing.T) {
	for uid, want := range map[string]float64{
		"claude-opus-4-6":         1.25,
		"claude-fable-5-2-high":   1.25,
		"gpt-5-6-sol":             1.25,
		"gpt-5-10":                1.25,
		"gpt-6-sol":               1.25,
		"gpt-6-2-astra-low":       1.25,
		"gpt-6-sol-high-priority": 1.25,
		"gpt-5-5":                 0,
		"gpt-7-0":                 0,
		"gemini-3-7-flash":        0,
		"swe-2-max":               0,
		"adaptive":                1.0,
	} {
		if got := devinCacheWriteRate(uid); got != want {
			t.Errorf("%s cache-write rate %v, want %v", uid, got, want)
		}
	}
}

// TestDevinFamilyCollapse checks the wire-lane projection: effort ladders show
// once as their logical family, service lanes join the -fast/-1m families, the
// bare face uid is never marked, fusion rows stay standalone, and a lone
// suffix-shaped uid keeps its own row.
func TestDevinFamilyCollapse(t *testing.T) {
	configs := [][]byte{
		devinTestConfig("swe-2-medium", "SWE-2 Medium", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(262144, 32768)) }),
		devinTestConfig("swe-2-high", "SWE-2 High", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(262144, 32768)) }),
		devinTestConfig("swe-2-max", "SWE-2 Max", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(262144, 32768)) }),
		devinTestConfig("swe-2-medium-lightning", "SWE-2 Medium Lightning", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(262144, 32768)) }),
		devinTestConfig("swe-2-high-lightning", "SWE-2 High Lightning", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(262144, 32768)) }),
		devinTestConfig("glm-5-2", "GLM 5.2", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(200000, 32000)) }),
		devinTestConfig("glm-5-2-max", "GLM 5.2 Max", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(200000, 32000)) }),
		devinTestConfig("glm-5-2-max-1m", "GLM 5.2 Max 1M", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(1000000, 32000)) }),
		devinTestConfig("glm-5-2-1m", "GLM 5.2 1M", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(1000000, 32000)) }),
		devinTestConfig("think-1-thinking", "Think 1 Thinking", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(128000, 16000)) }),
		devinTestConfig("think-1-no-thinking", "Think 1 No Thinking", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(128000, 16000)) }),
		devinTestConfig("bolt-fast", "Bolt Fast", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(128000, 16000)) }),
		devinTestConfig("bolt-lightning", "Bolt Lightning", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(128000, 16000)) }),
		devinTestConfig("fusion-swe-2-max-sidekick-glm-5-2", "Fusion", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(100000, 8000)) }),
		devinTestConfig("solo-9-low", "Solo Low", func(w *devinPbWriter) { w.appendMessage(23, devinTestFullInfo(64000, 8000)) }),
	}
	byID := devinNormalizeForTest(t, configs...)

	lane := func(uid string) string {
		model, ok := byID[uid]
		if !ok {
			t.Fatalf("%s missing from roster", uid)
		}
		var family string
		if raw := model.ProviderMeta.Compat.Extra["effortLane"]; raw != nil {
			_ = json.Unmarshal(raw, &family)
		}
		return family
	}
	if got := lane("swe-2-medium"); got != "swe-2" {
		t.Fatalf("swe-2-medium lane family %q", got)
	}
	if got := lane("swe-2-high-lightning"); got != "swe-2-fast" {
		t.Fatalf("swe-2-high-lightning lane family %q", got)
	}
	if got := lane("glm-5-2-max-1m"); got != "glm-5-2-1m" {
		t.Fatalf("glm-5-2-max-1m lane family %q", got)
	}
	// The bare 1m face is a family, not a lane.
	if got := lane("glm-5-2-1m"); got != "" {
		t.Fatalf("glm-5-2-1m wrongly marked as lane of %q", got)
	}
	if got := lane("glm-5-2"); got != "" {
		t.Fatalf("bare face glm-5-2 marked as lane %q", got)
	}
	// -no-thinking must match before -thinking, and both collapse under think-1.
	if got := lane("think-1-no-thinking"); got != "think-1" {
		t.Fatalf("think-1-no-thinking lane family %q, want think-1", got)
	}
	if got := lane("think-1-thinking"); got != "think-1" {
		t.Fatalf("think-1-thinking lane family %q, want think-1", got)
	}
	if got := byID["think-1"]; got == nil || got.DisplayName != "Think 1" {
		t.Fatalf("think-1 synthesized family = %+v", got)
	}
	if got := devinFamilyName("think-1", "Think 1 No Thinking"); got != "Think 1" {
		t.Fatalf("devinFamilyName(No Thinking) = %q, want Think 1", got)
	}
	// A bare -fast face keeps itself while -lightning collapses under it regardless of order.
	if got := lane("bolt-fast"); got != "" {
		t.Fatalf("bolt-fast wrongly marked as lane %q", got)
	}
	if got := lane("bolt-lightning"); got != "bolt-fast" {
		t.Fatalf("bolt-lightning lane family %q, want bolt-fast", got)
	}
	// The synthesized family routes through its strongest member.
	family, ok := byID["swe-2"]
	if !ok {
		t.Fatal("swe-2 family not synthesized")
	}
	var requestModel string
	_ = json.Unmarshal(family.ProviderMeta.Compat.Extra["requestModelId"], &requestModel)
	if requestModel != "swe-2-max" {
		t.Fatalf("family requestModelId %q", requestModel)
	}
	if _, marked := family.ProviderMeta.Compat.Extra["effortLane"]; marked {
		t.Fatal("synthesized family carries a lane marker")
	}
	var routing map[string]string
	if err := json.Unmarshal(family.ProviderMeta.Compat.Extra["effortRouting"], &routing); err != nil {
		t.Fatalf("effortRouting: %v", err)
	}
	if routing["medium"] != "swe-2-medium" || routing["max"] != "swe-2-max" {
		t.Fatalf("effort routing %#v", routing)
	}
	fast, ok := byID["swe-2-fast"]
	if !ok {
		t.Fatal("swe-2-fast family not synthesized")
	}
	_ = json.Unmarshal(fast.ProviderMeta.Compat.Extra["requestModelId"], &requestModel)
	if requestModel != "swe-2-high-lightning" {
		t.Fatalf("fast family requestModelId %q", requestModel)
	}
	// Fusion pairings and lone suffix uids stay their own rows.
	if _, ok := byID["fusion-swe-2-max-sidekick-glm-5-2"]; !ok {
		t.Fatal("fusion pairing lost in collapse")
	}
	if got := lane("solo-9-low"); got != "" {
		t.Fatalf("orphan suffix uid absorbed into %q", got)
	}
	if _, ok := byID["solo-9"]; ok {
		t.Fatal("phantom family synthesized for an orphan lane")
	}
	// Re-projecting a stored roster is idempotent and stays sorted.
	reprojected := devinCollapseFamilies([]*Model{
		{ID: "swe-2-medium", DisplayName: "SWE-2 Medium"},
		{ID: "swe-2-max", DisplayName: "SWE-2 Max"},
	})
	if len(reprojected) != 3 || reprojected[0].ID != "swe-2" || reprojected[1].ID != "swe-2-max" || reprojected[2].ID != "swe-2-medium" {
		t.Fatalf("reprojected stored roster = %+v", reprojected)
	}
	if again := devinCollapseFamilies(reprojected); len(again) != 3 {
		t.Fatalf("re-projecting collapsed roster duplicated entries: %d", len(again))
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

	// Shared endpoint metadata alone (apiServerUrl without an account/org/user
	// discriminator) must stay ephemeral so distinct users never share a key.
	serverOnlyA := provider.ModelCatalog.Scope(provider.BaseURL, &Credential{
		Type:   CredentialOAuth,
		Access: "token-a",
		Extra:  map[string]json.RawMessage{"apiServerUrl": json.RawMessage(`"https://server.codeium.com"`)},
	})
	serverOnlyB := provider.ModelCatalog.Scope(provider.BaseURL, &Credential{
		Type:   CredentialOAuth,
		Access: "token-b",
		Extra:  map[string]json.RawMessage{"apiServerUrl": json.RawMessage(`"https://server.codeium.com"`)},
	})
	if serverOnlyA.StoreKey != "" || serverOnlyB.StoreKey != "" {
		t.Fatalf("apiServerUrl-only scope persisted: %+v vs %+v", serverOnlyA, serverOnlyB)
	}
	if serverOnlyA.ID == "" || serverOnlyA.ID == serverOnlyB.ID {
		t.Fatalf("apiServerUrl-only accounts collided: %+v vs %+v", serverOnlyA, serverOnlyB)
	}
}
