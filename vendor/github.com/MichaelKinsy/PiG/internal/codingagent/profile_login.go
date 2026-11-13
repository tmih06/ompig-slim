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

// SyncedDocuments is installed by the embedding CLI before interactive startup,
// beside ProfileLogin, and read once when a mode begins. A receive means the
// embedder's synchronization worker rewrote a profile document; the mode then
// re-reads the configuration it loaded. The sender re-reads from disk, so a
// coalesced signal costs one redundant read and must never be sent as lost
// information. Nil leaves configuration immutable until /reload, as upstream.
var SyncedDocuments <-chan struct{}
