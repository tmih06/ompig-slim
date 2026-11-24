package sessionentry

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/extensions/sdk/json"
)

func TestCompactionEntryInfoRoundTrip(t *testing.T) {
	info := &agent.CompactionInfo{Reason: "idle", ElapsedMs: 2400, Models: []string{"fixture/reported-chat"}}
	entry := CompactionEntry{
		SessionEntryBase: SessionEntryBase{Type: "compaction", ID: "deadbeef", ParentID: nil, Timestamp: "2026-01-01T00:00:00.000Z"},
		Summary:          "summary",
		FirstKeptEntryID: "deadbeef",
		TokensBefore:     87432,
		Info:             info,
	}
	encoded, err := entry.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"info":{"reason":"idle","elapsedMs":2400,"models":["fixture/reported-chat"]}`) {
		t.Fatalf("encoded entry lost or reordered info: %s", encoded)
	}
	decoded := DecodeSessionEntry(encoded)
	compaction, ok := decoded.(CompactionEntry)
	if !ok {
		t.Fatalf("decoded %T, want CompactionEntry", decoded)
	}
	if compaction.Info == nil || compaction.Info.Reason != "idle" || compaction.Info.ElapsedMs != 2400 ||
		len(compaction.Info.Models) != 1 || compaction.Info.Models[0] != "fixture/reported-chat" {
		t.Fatalf("decoded info = %+v", compaction.Info)
	}
}

func TestCompactionEntryWithoutInfoWritesNoMember(t *testing.T) {
	entry := CompactionEntry{
		SessionEntryBase: SessionEntryBase{Type: "compaction", ID: "deadbeef", ParentID: nil, Timestamp: "2026-01-01T00:00:00.000Z"},
		Summary:          "summary",
		FirstKeptEntryID: "deadbeef",
		TokensBefore:     87432,
	}
	encoded, err := entry.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"info"`) {
		t.Fatalf("entry without info wrote an info member: %s", encoded)
	}
	decoded := DecodeSessionEntry(encoded)
	compaction, ok := decoded.(CompactionEntry)
	if !ok {
		t.Fatalf("decoded %T, want CompactionEntry", decoded)
	}
	if compaction.Info != nil {
		t.Fatalf("decoded info = %+v, want nil", compaction.Info)
	}
	// The decoded entry rewrites the record it came from.
	reencoded, err := compaction.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(reencoded) != string(encoded) {
		t.Fatalf("record did not round trip:\n got %s\nwant %s", reencoded, encoded)
	}
}

func TestCompactionEntryInfoSurvivesDecodedRecord(t *testing.T) {
	// A compaction entry decoded from a record and written back unchanged is
	// the retained-raw path the session log rewrites through.
	raw := json.RawMessage(`{"type":"compaction","id":"deadbeef","parentId":null,"timestamp":"2026-01-01T00:00:00.000Z","summary":"summary","firstKeptEntryId":"deadbeef","tokensBefore":100,"fromHook":false,"info":{"reason":"manual","elapsedMs":12,"models":["fixture/chat"]}}`)
	decoded := DecodeSessionEntry(raw)
	compaction, ok := decoded.(CompactionEntry)
	if !ok {
		t.Fatalf("decoded %T, want CompactionEntry", decoded)
	}
	if compaction.Info == nil || compaction.Info.Reason != "manual" {
		t.Fatalf("decoded info = %+v", compaction.Info)
	}
	encoded, err := compaction.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(raw) {
		t.Fatalf("retained record changed:\n got %s\nwant %s", encoded, raw)
	}
}
