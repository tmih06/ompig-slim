package goalext

import (
	"context"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/coding/extension/factoryload"
	"github.com/MichaelKinsy/PiG/coding/extension/host/inproc"
)

// TestGoalSubmitsSkillObjective exercises the real command runner. Goal must
// forward slash skill syntax to the host's normal prompt/resource expansion.
func TestGoalSubmitsSkillObjective(t *testing.T) {
	for _, args := range []string{"/skill:review fix this", "set /skill:review fix this", "create /skill:review fix this"} {
		t.Run(args, func(t *testing.T) {
			root := t.TempDir()
			loaded, err := factoryload.LoadExtensionFromFactory(Extension, root, extension.CreateEventBus(), extension.CreateExtensionRuntime(), "<inline:goal>")
			if err != nil {
				t.Fatal(err)
			}
			runner := inproc.NewRunner([]extension.Extension{loaded}, root)
			var submitted any
			runner.BindCore(extension.ExtensionActions{SendUserMessage: func(content any, _ *extension.SendUserMessageOptions) error { submitted = content; return nil }}, extension.ContextActions{}, nil)
			if !runner.ExecuteCommand(context.Background(), "goal", args) {
				t.Fatal("goal command not handled")
			}
			if submitted != "/skill:review fix this" {
				t.Fatalf("submitted = %q; want skill objective", submitted)
			}
		})
	}
}
