package subagents

// Outcome is the machine-readable terminal state a caller acts on. Record.Status
// stays the persisted source of truth; Outcome names what each status means so
// no caller re-derives it from a string.
type Outcome string

const (
	// OutcomeRunning covers queued, running, and scheduled children.
	OutcomeRunning Outcome = "running"
	// OutcomeCompleted is a child that finished a normal turn, including one
	// that wrapped up inside its grace turns after the soft turn limit.
	OutcomeCompleted Outcome = "completed"
	// OutcomeFailed is a child that stopped on an error.
	OutcomeFailed Outcome = "failed"
	// OutcomeCanceled is a child a person or its parent stopped. A parent that
	// cancels its own wait sees its own cancellation and does not create this.
	OutcomeCanceled Outcome = "canceled"
	// OutcomeExhausted is a child the hard turn budget stopped after its grace
	// turns, which is distinct from a cancellation.
	OutcomeExhausted Outcome = "exhausted"
)

var outcomesByStatus = map[string]Outcome{
	"queued":    OutcomeRunning,
	"running":   OutcomeRunning,
	"scheduled": OutcomeRunning,
	"completed": OutcomeCompleted,
	"steered":   OutcomeCompleted,
	"error":     OutcomeFailed,
	"stopped":   OutcomeCanceled,
	"aborted":   OutcomeCanceled,
	"exhausted": OutcomeExhausted,
}

func (r Record) Outcome() Outcome {
	if outcome, ok := outcomesByStatus[r.Status]; ok {
		return outcome
	}
	// An unknown status is a nonterminal record from a newer or hand-edited
	// file. Reporting it as running keeps it pollable instead of calling it a
	// success the caller never observed.
	return OutcomeRunning
}

// OutcomeEnvelope is the machine-readable result the subagent tools attach as
// tool details, so a caller reads the outcome instead of inferring it from text
// that may legitimately be empty.
type OutcomeEnvelope struct {
	AgentID string  `json:"agentId"`
	Outcome Outcome `json:"outcome"`
	Partial bool    `json:"partial"`
	Error   string  `json:"error,omitempty"`
}

func (r Record) Envelope() OutcomeEnvelope {
	envelope := OutcomeEnvelope{AgentID: r.ID, Outcome: r.Outcome(), Partial: r.Partial()}
	if r.Error != "" && r.Error != r.Result {
		envelope.Error = r.Error
	}
	return envelope
}

// retrievalText is what a caller reads in place of the result. A completed child
// returns its exact text, so an intentionally empty answer stays empty; every
// incomplete outcome states itself instead of leaving an empty result to be read
// as success.
func retrievalText(r Record) string {
	if r.Outcome() == OutcomeCompleted {
		return r.Result
	}
	text := "subagent_outcome: " + string(r.Outcome())
	if r.Error != "" && r.Error != r.Result {
		text += "\nerror: " + toolResultPreview(r.Error)
	}
	if r.Outcome() == OutcomeRunning {
		return text
	}
	if r.Result == "" {
		return text + "\npartial_output: none"
	}
	return text + "\npartial_output: incomplete, the agent did not finish\n" + r.Result
}

// Partial reports text a child produced before an incomplete stop. Successful
// completion is never partial, and an incomplete child with no text has no
// partial answer to offer.
func (r Record) Partial() bool {
	return r.Outcome() != OutcomeCompleted && r.Outcome() != OutcomeRunning && r.Result != ""
}
