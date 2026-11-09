package todotool

import (
	"encoding/json"

	"github.com/MichaelKinsy/PiG/ai"

	"ompig-slim/internal/todo"
)

// todoDetails is one card's result to draw in a test.
type todoDetails struct {
	phases []todo.Phase
	op     todo.Op
}

func (d todoDetails) value() todo.Details {
	op := d.op
	if op == "" {
		op = todo.OpInit
	}
	return todo.Details{Op: op, Phases: d.phases}
}

func (d todoDetails) content() []ai.ToolResultMessageContent {
	return []ai.ToolResultMessageContent{ai.TextContent{Text: todo.Summary(d.phases, nil, false)}}
}

// args are the call arguments the card reads to decide which phases it touched.
func (d todoDetails) args() json.RawMessage {
	op := d.op
	if op == "" {
		op = todo.OpInit
	}
	raw, _ := json.Marshal(map[string]any{"op": string(op)})
	return raw
}
