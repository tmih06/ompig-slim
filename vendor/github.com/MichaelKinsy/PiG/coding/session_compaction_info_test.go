package coding

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
)

// fixedCompleter is a text-only SimpleCompleter: it reports no provider
// response identity, so a successful summary attributes the resolved
// request, not a reported backend.
type fixedCompleter struct {
	text string
}

func (c fixedCompleter) CompleteSimple(ctx context.Context, model *ai.Model, systemPrompt string, messages []agent.AgentMessage, options ai.StreamOptions) (string, *ai.Usage, error) {
	return c.text, &ai.Usage{}, nil
}

// The collector's identity rules: a validated response reports its own
// provider/model, and a response or text-only request without one keeps the
// resolved request's provider/model rather than dropping the model half.
func TestCompactionModelCollectorIdentity(t *testing.T) {
	request := &ai.Model{ID: "resolved", ProviderMeta: ai.ProviderMetadata{ProviderID: "fixture"}}

	var reported compactionModelCollector
	reported.observe(request, &ai.AssistantMessage{Provider: "backend", ResponseModel: "served"})
	if got := reported.identities(); !reflect.DeepEqual(got, []string{"backend/served"}) {
		t.Fatalf("reported identity = %v", got)
	}

	var anonymous compactionModelCollector
	anonymous.observe(request, &ai.AssistantMessage{})
	if got := anonymous.identities(); !reflect.DeepEqual(got, []string{"fixture/resolved"}) {
		t.Fatalf("anonymous response identity = %v, want the resolved request's", got)
	}

	var textOnly compactionModelCollector
	textOnly.addRequest(request)
	if got := textOnly.identities(); !reflect.DeepEqual(got, []string{"fixture/resolved"}) {
		t.Fatalf("text-only identity = %v, want the resolved request's", got)
	}
}

// A text-only completer attributes its summary to the resolved request
// target: the compact-role model, never the chat selection or the start
// label.
func TestSessionCompactionInfoTextCompleter(t *testing.T) {
	tc := compactionFallbackCase{thirdHealthy: true}
	fixture := newCompactionFallbackFixture(t, tc, []string{"fallback-b/second:off", "fallback-c/third:off"}, false)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, prompt := range []string{
		"older conversation details for summary",
		"recent conversation details to retain",
	} {
		if _, err := fixture.session.Send(ctx, prompt); err != nil {
			t.Fatal(err)
		}
	}
	fixture.session.completer = fixedCompleter{text: "completer summary"}
	result, err := fixture.session.Compact(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "completer summary" {
		t.Fatalf("summary = %q", result.Summary)
	}
	assertCompactionInfo(t, fixture, result, "manual", []string{"fallback-a/primary"})
}

// The host did not observe an extension's direct append, so the entry
// carries no info record and the record's members stay untouched.
func TestSessionCompactionAppendLegacyWritesNoInfo(t *testing.T) {
	tc := compactionFallbackCase{thirdHealthy: true}
	fixture := newCompactionFallbackFixture(t, tc, []string{"fallback-b/second:off", "fallback-c/third:off"}, false)
	id, err := fixture.session.Inner().AppendCompaction("external summary", "", 100, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	entry, found := fixture.session.Inner().GetEntry(id)
	if !found {
		t.Fatal("appended entry not found")
	}
	compaction, ok := entry.(icodingagent.CompactionEntry)
	if !ok {
		t.Fatalf("entry is %T, want CompactionEntry", entry)
	}
	if compaction.Info != nil {
		t.Fatalf("extension append invented info: %+v", compaction.Info)
	}
	if raw := string(entry.Raw()); strings.Contains(raw, `"info"`) {
		t.Fatalf("append without info wrote an info member: %s", raw)
	}
}
