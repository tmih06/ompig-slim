package subagents

import "testing"

func BenchmarkTerminalResultRequest(b *testing.B) {
	m := New(Host{CWD: b.TempDir(), AgentDir: b.TempDir(), Restore: func() []Record {
		return []Record{{ID: "agent_bench", Status: "completed", Result: "exact answer"}}
	}})
	b.Cleanup(m.Close)
	b.ReportAllocs()
	for b.Loop() {
		r, disposition, err := m.RequestResult("agent_bench")
		if err != nil || disposition != resultReady || r.Result != "exact answer" {
			b.Fatalf("terminal result: %q, disposition %v, error %v", r.Result, disposition, err)
		}
	}
}
