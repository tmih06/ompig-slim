package codingagent

import (
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

type costPolicyBranch struct {
	CacheWarmerSessionManager
	tokens int
}

func (b costPolicyBranch) GetBranch(...string) []SessionEntry {
	return []SessionEntry{MessageEntry{SessionEntryBase: SessionEntryBase{Type: "message"}, Message: agent.AgentMessage{Assistant: &agent.AssistantMessage{Usage: &ai.Usage{Input: b.tokens}}}}}
}

func TestCostPolicyPreservesProfitableCacheWarming(t *testing.T) {
	if mode := NewInMemorySettingsManager(Settings{}).GetCacheWarmingMode(); mode != "streaming" {
		t.Fatalf("cost-saving cache warming was disabled: %s", mode)
	}
	for _, test := range []struct {
		name, phase string
		tokens      int
		input, read float64
		want        CacheWarmingAction
	}{
		{"profitable streaming", "streaming", 100000, 3, .3, CacheWarmingActionWarm},
		{"unlikely idle continuation", "idle", 100000, 3, .3, CacheWarmingActionStop},
		{"profitable idle", "idle", 1000000, 3, .3, CacheWarmingActionWarm},
		{"no cache discount", "streaming", 100000, 3, 3, CacheWarmingActionStop},
		{"unknown prompt size", "streaming", 0, 3, .3, CacheWarmingActionStop},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := CacheWarmer{sessionManager: costPolicyBranch{tokens: test.tokens}}
			run := &cacheWarmingRun{CacheWarmRequest: CacheWarmRequest{Model: &ai.Model{Capabilities: ai.ModelCapabilities{InputCostPer1M: test.input, CacheReadCostPer1M: test.read, OutputCostPer1M: 15}}}, phase: test.phase}
			d := w.evaluate(run)
			if d.Action != test.want {
				t.Fatalf("action=%s; expected %s (estimated savings $%f)", d.Action, test.want, d.ExpectedSavings)
			}
		})
	}
}
