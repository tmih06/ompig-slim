package codingagent

import (
	"context"

	"github.com/MichaelKinsy/PiG/ai"
)

// ProfileLoginOption adds a storage-backend setup flow to /login, without
// registering a model provider or saving its secret in synchronized credentials.
// Login runs on the joined dialog worker and must honor prompt cancellation.
type ProfileLoginOption struct {
	ID    string
	Name  string
	Login func(context.Context, string, ai.AuthInteraction) error
}

// ProfileLogin is installed by the embedding CLI before interactive startup.
// It must not be changed while a dialog or runtime is running.
var ProfileLogin *ProfileLoginOption
