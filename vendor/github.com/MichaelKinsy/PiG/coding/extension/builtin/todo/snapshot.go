package todo

import (
	"encoding/json"

	"github.com/MichaelKinsy/PiG/coding/extension"
)

// phasesOf decodes one session entry into a todo snapshot, reporting whether the
// entry carries one that is authoritative.
//
// An entry counts only when it is a `todo` tool result that succeeded and was not
// a `view`. A `view` echoes the list without changing it, so a later view must not
// out-rank the mutation it read; an errored result persisted nothing, so treating
// it as state would resurrect a rejected batch on the next call.
//
// The details are decoded through JSON rather than a type assertion because the
// branch's entries are read back from the session file, where details arrive as
// decoded JSON values, not as the Details the tool wrote.
func phasesOf(entry extension.SessionEntry) ([]Phase, bool) {
	if entry == nil {
		return nil, false
	}
	raw := entry.Raw()
	if len(raw) == 0 {
		return nil, false
	}
	var record struct {
		Message *struct {
			Role     string `json:"role"`
			ToolName string `json:"toolName"`
			IsError  bool   `json:"isError"`
			Details  *struct {
				Op     Op       `json:"op"`
				Phases *[]Phase `json:"phases"`
			} `json:"details"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &record) != nil || record.Message == nil {
		return nil, false
	}
	message := record.Message
	if message.Role != "toolResult" || message.ToolName != ToolName || message.IsError {
		return nil, false
	}
	if message.Details == nil || message.Details.Phases == nil {
		return nil, false
	}
	if message.Details.Op == OpView {
		return nil, false
	}
	return ClonePhases(*message.Details.Phases), true
}

// latestPhases returns the newest todo snapshot on the branch that a stop-time
// check should judge by, or nil when the branch holds none.
//
// The reminder and the tool share this reader so they can never disagree about
// the list: the reminder enforces exactly what the model's next call will see.
func latestPhases(entries []extension.SessionEntry) []Phase {
	for i := len(entries) - 1; i >= 0; i-- {
		if phases, ok := phasesOf(entries[i]); ok {
			return phases
		}
	}
	return nil
}
