package inproc

import (
	"context"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
)

// TestEmptyBoundaryStillProjects preserves the public runner's original
// no-handler result. Only explicit summary subscriptions may skip projection.
func TestEmptyBoundaryStillProjects(t *testing.T) {
	runner := NewRunner(nil, t.TempDir())
	defer runner.Shutdown()
	builds := 0
	base := extension.TurnEndEvent{Type: "turn_end", BoundaryState: &extension.BoundaryState{Outcome: extension.AgentActivityCompleted}}
	result, err := runner.EmitBoundary(context.Background(), base, func([]extension.SessionBoundaryDraft) (extension.BoundaryContextPreview, error) {
		builds++
		return extension.BoundaryContextPreview{CanContinue: true}, nil
	})
	if err != nil || !result.Valid || !result.Context.CanContinue || builds != 1 {
		t.Fatal(result, err, builds)
	}
}
