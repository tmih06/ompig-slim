package agent

// CompactionInfo is the host-observed record of one completed compaction:
// why it ran, how long the claimed operation took, and which provider/model
// identities produced the summary. It is frozen once the summary is
// validated and is shared unchanged by the persisted entry, the
// compaction_end event and the operation's result.
//
// A nil Info means the host did not observe the operation: an entry from an
// older build or one an extension appended directly. An empty Models means
// the host observed the operation but cannot attribute the summary to a
// native model, such as a session_before_compact hook summary.
//
// Reason carries the same strings as CompactionStartEvent ("manual",
// "threshold", "overflow", "idle"); an unknown future reason is preserved,
// not coerced. ElapsedMs spans the operation from its start event until its
// validated summary entered the commit path: extension hooks, auth routing,
// retries and fallback count, idle and admission waits and persistence do
// not. Each model string is the effective provider/model identity, in
// encounter order, preferring the response's reported model over the
// resolved request's.
type CompactionInfo struct {
	Reason    string   `json:"reason"`
	ElapsedMs int64    `json:"elapsedMs"`
	Models    []string `json:"models,omitempty"`
}
