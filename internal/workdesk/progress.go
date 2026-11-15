package workdesk

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/durable"
)

const (
	progressKind        = "ompig.progress"
	progressMaxItems    = 16
	progressMaxText     = 500
	progressMaxRevision = 1_000_000
)

// ProgressCheckpoint is the latest committed report for one immutable assignment.
// CompletedWork is an agent claim; it does not imply acceptance or verified tests.
type ProgressCheckpoint struct {
	AssignmentRequestID string   `json:"assignmentRequestId"`
	Revision            int      `json:"revision"`
	Criteria            []string `json:"criteria"`
	CompletedWork       []string `json:"completedWork"`
	CurrentStep         string   `json:"currentStep"`
	Constraints         []string `json:"constraints"`
	Decisions           []string `json:"decisions"`
	Blockers            []string `json:"blockers"`
	NextAction          string   `json:"nextAction"`
}

type ProgressView struct {
	Present    bool                `json:"present"`
	Checkpoint *ProgressCheckpoint `json:"checkpoint,omitempty"`
}

type progressUpdate struct {
	Criteria      []string `json:"criteria"`
	CompletedWork []string `json:"completedWork"`
	CurrentStep   string   `json:"currentStep"`
	Constraints   []string `json:"constraints"`
	Decisions     []string `json:"decisions"`
	Blockers      []string `json:"blockers"`
	NextAction    string   `json:"nextAction"`
}

var progressDoc = durable.DefineDoc(durable.DocDefinition[ProgressCheckpoint]{
	Kind:    progressKind,
	Version: 1,
	Initial: func() ProgressCheckpoint {
		return ProgressCheckpoint{Criteria: []string{}, CompletedWork: []string{}, Constraints: []string{}, Decisions: []string{}, Blockers: []string{}}
	},
	CheckpointWhenRaw: func(durable.JsonObject, []durable.Op, durable.CheckpointInfo) bool { return true },
	DocumentSemantics: durable.DocumentSemantics{Scope: durable.ScopeConversation, History: durable.HistoryLatest, Fork: durable.ForkCurrent},
})

func newProgressExtension(job assignment) *durable.Extension {
	return &durable.Extension{
		Name:  "ompig-progress",
		Tools: []*durable.ToolRegistration{progressTool(job)},
		Sections: []*durable.PromptSection{{
			Key: "ompig_progress",
			Render: func(ctx context.Context, input durable.PromptInput) (*string, error) {
				checkpoint, err := durable.Snapshot[ProgressCheckpoint](ctx, input.Read, progressDoc, input.ConversationId)
				if err != nil || checkpoint == nil {
					if err != nil {
						return nil, err
					}
					message := "No progress checkpoint has been committed."
					return &message, nil
				}
				raw, err := json.Marshal(checkpoint)
				if err != nil {
					return nil, err
				}
				message := "Authoritative committed progress checkpoint (agent claims, not acceptance): " + string(raw)
				return &message, nil
			},
		}},
	}
}

func progressTool(job assignment) *durable.ToolRegistration {
	return &durable.ToolRegistration{
		ToolSchema: ai.ToolSchema{
			Name:        "update_progress",
			Description: "Commit the bounded progress checkpoint for this assignment after a meaningful milestone. Completed work is an agent claim, not proof that criteria passed.",
			Parameters: map[string]any{"type": "object", "additionalProperties": false, "required": []any{"criteria", "completedWork", "currentStep", "constraints", "decisions", "blockers", "nextAction"}, "properties": map[string]any{
				"criteria":      map[string]any{"type": "array", "maxItems": progressMaxItems, "items": map[string]any{"type": "string", "maxLength": progressMaxText}},
				"completedWork": map[string]any{"type": "array", "maxItems": progressMaxItems, "items": map[string]any{"type": "string", "maxLength": progressMaxText}},
				"currentStep":   map[string]any{"type": "string", "maxLength": progressMaxText},
				"constraints":   map[string]any{"type": "array", "maxItems": progressMaxItems, "items": map[string]any{"type": "string", "maxLength": progressMaxText}},
				"decisions":     map[string]any{"type": "array", "maxItems": progressMaxItems, "items": map[string]any{"type": "string", "maxLength": progressMaxText}},
				"blockers":      map[string]any{"type": "array", "maxItems": progressMaxItems, "items": map[string]any{"type": "string", "maxLength": progressMaxText}},
				"nextAction":    map[string]any{"type": "string", "maxLength": progressMaxText},
			}},
		},
		Callers: []durable.ToolCaller{durable.CallerModel},
		Replay:  durable.ReplaySafe,
		Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
			update, err := durable.FromJsonValue[progressUpdate](args)
			if err != nil {
				return durable.ToolExecutionResult{}, err
			}
			if err = validateProgressUpdate(update); err != nil {
				return durable.ToolExecutionResult{}, err
			}
			_, err = api.Commit(ctx, func(tx durable.Tx) (any, error) {
				draft, err := durable.TxDoc(tx, progressDoc, api.ConversationId())
				if err != nil {
					return nil, err
				}
				checkpoint, err := durable.DecodeDoc[ProgressCheckpoint](draft.Snapshot())
				if err != nil {
					return nil, err
				}
				if checkpoint.AssignmentRequestID != "" && checkpoint.AssignmentRequestID != job.RequestID {
					return nil, fmt.Errorf("progress belongs to another assignment")
				}
				if checkpoint.Revision >= progressMaxRevision {
					return nil, fmt.Errorf("progress revision limit reached")
				}
				values := map[string]any{
					"assignmentRequestId": job.RequestID, "revision": checkpoint.Revision + 1,
					"criteria": update.Criteria, "completedWork": update.CompletedWork, "currentStep": update.CurrentStep,
					"constraints": update.Constraints, "decisions": update.Decisions, "blockers": update.Blockers,
					"nextAction": update.NextAction,
				}
				for key, value := range values {
					if err := draft.Set(key, value); err != nil {
						return nil, err
					}
				}
				return nil, nil
			})
			if err != nil {
				return durable.ToolExecutionResult{}, err
			}
			return durable.ToolExecutionResult{Output: []ai.ToolResultMessageContent{ai.TextContent{Text: "Progress checkpoint committed."}}}, nil
		},
	}
}

func validateProgressUpdate(update progressUpdate) error {
	for _, values := range [][]string{update.Criteria, update.CompletedWork, update.Constraints, update.Decisions, update.Blockers} {
		if len(values) > progressMaxItems {
			return fmt.Errorf("progress lists may contain at most %d items", progressMaxItems)
		}
		for _, value := range values {
			if len([]rune(value)) > progressMaxText {
				return fmt.Errorf("progress text exceeds %d characters", progressMaxText)
			}
		}
	}
	for name, value := range map[string]string{"currentStep": update.CurrentStep, "nextAction": update.NextAction} {
		if len([]rune(value)) > progressMaxText {
			return fmt.Errorf("progress %s exceeds %d characters", name, progressMaxText)
		}
	}
	return nil
}

func readProgress(ctx context.Context, storage durable.Storage, conversationID durable.ConversationId, requestID string) (ProgressView, error) {
	address := durable.DocumentAddress{Kind: progressKind, Scope: durable.DocumentRecordScope{Kind: durable.ScopeConversation, ConversationId: conversationID}}
	record, err := storage.FindDocument(ctx, address, durable.CurrentPoint)
	if err != nil || record == nil {
		return ProgressView{}, err
	}
	stored, err := storage.Document(ctx, record.Id, durable.CurrentPoint)
	if err != nil || stored == nil {
		return ProgressView{}, err
	}
	value, err := durable.MaterializeDocument(progressDoc.AnyDefinition(), *stored)
	if err != nil {
		return ProgressView{}, err
	}
	checkpoint, err := durable.DecodeDoc[ProgressCheckpoint](value)
	if err != nil {
		return ProgressView{}, err
	}
	if checkpoint.AssignmentRequestID != requestID {
		return ProgressView{}, fmt.Errorf("progress belongs to another assignment")
	}
	return ProgressView{Present: true, Checkpoint: checkpoint}, nil
}
