package todo

import (
	"encoding/json"

	"github.com/MichaelKinsy/PiG/ai"
)

// todoDetails is one card's result to draw in a test.
type todoDetails struct {
	phases []Phase
	op     Op
}

func (d todoDetails) value() Details {
	op := d.op
	if op == "" {
		op = OpInit
	}
	return Details{Op: op, Phases: d.phases}
}

func (d todoDetails) content() []ai.ToolResultMessageContent {
	return []ai.ToolResultMessageContent{ai.TextContent{Text: Summary(d.phases, nil, false)}}
}

// args are the call arguments the card reads to decide which phases it touched.
func (d todoDetails) args() json.RawMessage {
	op := d.op
	if op == "" {
		op = OpInit
	}
	raw, _ := json.Marshal(map[string]any{"op": string(op)})
	return raw
}
