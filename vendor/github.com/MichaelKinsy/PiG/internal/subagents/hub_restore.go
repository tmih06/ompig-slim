package subagents

import (
	"bufio"
	"context"
	"encoding/json"
	"os"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

// Recover tool/response activity from saved child messages without reopening an
// agent or spending a model turn. Scan once at restore, never during rendering;
// the scratch manager retains only the same bounded activity ring as a live run.
func readHubActivity(ctx context.Context, path, id string) ([]HubActivity, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)
	var scratch Manager
	r := &run{Record: Record{ID: id}}
	index := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var entry struct {
			Type    string          `json:"type"`
			Message json.RawMessage `json:"message"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Type != "message" {
			continue
		}
		var message agent.AgentMessage
		if json.Unmarshal(entry.Message, &message) != nil {
			continue
		}
		add := func(kind, title, summary, status string, timestamp int64) int {
			slot := scratch.hubEventLocked(r, kind, title, summary, status, index)
			scratch.hubActivity[slot].Timestamp = timestamp
			return slot
		}
		if a := message.Assistant; a != nil {
			for _, block := range a.Content {
				if call, ok := block.(ai.ToolCall); ok {
					args, _ := call.ArgumentsJSON()
					slot := add("tool", call.Name, string(args), "aborted", a.Timestamp)
					if scratch.hubTools == nil {
						scratch.hubTools = map[string]int{}
					}
					key := id + ":" + call.ID
					scratch.hubTools[key] = slot
					scratch.hubActivity[slot].toolKey = key
				}
			}
			if a.StopReason == "error" {
				add("response", "Error", a.ErrorMessage, "error", a.Timestamp)
			}
			if text := assistantText(a); text != "" {
				add("response", "Response", text, "completed", a.Timestamp)
			}
		}
		if result := message.ToolResult; result != nil {
			if _, ok := scratch.hubTools[id+":"+result.ToolCallID]; !ok {
				slot := add("tool", result.ToolName, "", "aborted", result.Timestamp)
				if scratch.hubTools == nil {
					scratch.hubTools = map[string]int{}
				}
				key := id + ":" + result.ToolCallID
				scratch.hubTools[key] = slot
				scratch.hubActivity[slot].toolKey = key
			}
			scratch.recordHubEventLocked(r, agent.ToolExecutionEndEvent{ToolCallID: result.ToolCallID, ToolName: result.ToolName, IsError: result.IsError, Result: agent.AgentToolResult{Content: result.Content}})
		}
		index++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return scratch.HubActivity(), nil
}
