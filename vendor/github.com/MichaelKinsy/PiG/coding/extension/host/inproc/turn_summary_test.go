package inproc

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
)

// TestTurnSummarySkipsHistory pins the absence of full-history work for a
// no-draft turn, including handler order and snapshot unsubscribe semantics.
func TestTurnSummarySkipsHistory(t *testing.T) {
	var ext extension.Extension
	var order []int
	ext.AddTurnSummaryHandler(1, func(args ...any) (any, error) {
		event := args[0].(extension.TurnEndEvent)
		if !reflect.DeepEqual(event.Context, extension.BoundaryContextPreview{}) || event.Outcome != extension.AgentActivityCompleted {
			t.Fatal("summary exposed context or lost outcome", event)
		}
		order = append(order, 1)
		ext.RemoveEventHandler("turn_end", 2)
		return nil, nil
	})
	ext.AddTurnSummaryHandler(2, func(...any) (any, error) {
		order = append(order, 2)
		return nil, nil
	})
	runner := NewRunner([]extension.Extension{ext}, t.TempDir())
	defer runner.Shutdown()
	builds := 0
	build := func([]extension.SessionBoundaryDraft) (extension.BoundaryContextPreview, error) {
		builds++
		return extension.BoundaryContextPreview{}, nil
	}
	base := extension.TurnEndEvent{Type: "turn_end", BoundaryState: &extension.BoundaryState{Outcome: extension.AgentActivityCompleted}}
	for i := 0; i < 2; i++ {
		result, err := runner.EmitBoundary(context.Background(), base, build)
		if err != nil || !result.Valid || len(result.Entries) != 0 {
			t.Fatal(result, err)
		}
	}
	if builds != 0 || !reflect.DeepEqual(order, []int{1, 2, 1}) {
		t.Fatal("unexpected projection work or registration order", builds, order)
	}
}

// TestTurnSummaryValidatesDrafts ensures the optimization cannot bypass invalid
// draft detection, repair by later handlers, or ordinary full-context delivery.
func TestTurnSummaryValidatesDrafts(t *testing.T) {
	var ext extension.Extension
	entries := []extension.SessionBoundaryDraft{{CustomType: "invalid"}}
	ext.AddTurnSummaryHandler(1, func(...any) (any, error) {
		return extension.BoundaryResult{Entries: &entries}, nil
	})
	ext.AddTurnSummaryHandler(2, func(args ...any) (any, error) {
		event := args[0].(extension.TurnEndEvent)
		if len(event.Entries) != 1 {
			t.Fatal("lost previous invalid proposal")
		}
		empty := []extension.SessionBoundaryDraft{}
		return extension.BoundaryResult{Entries: &empty}, nil
	})
	ext.AddEventHandler("turn_end", 3, func(args ...any) (any, error) {
		event := args[0].(extension.TurnEndEvent)
		if !event.Context.CanContinue || len(event.Entries) != 0 {
			t.Fatal("ordinary handler did not get repaired projection", event)
		}
		return nil, nil
	})
	runner := NewRunner([]extension.Extension{ext}, t.TempDir())
	defer runner.Shutdown()
	builds := 0
	base := extension.TurnEndEvent{Type: "turn_end", BoundaryState: &extension.BoundaryState{Outcome: extension.AgentActivityCompleted}}
	result, err := runner.EmitBoundary(context.Background(), base, func(drafts []extension.SessionBoundaryDraft) (extension.BoundaryContextPreview, error) {
		builds++
		if len(drafts) > 0 {
			return extension.BoundaryContextPreview{}, errors.New("invalid draft")
		}
		return extension.BoundaryContextPreview{CanContinue: true}, nil
	})
	if err != nil || !result.Valid || len(result.Entries) != 0 || builds != 3 {
		t.Fatal(result, err, builds)
	}
}
