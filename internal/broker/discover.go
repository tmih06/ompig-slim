package broker

import (
	"path/filepath"

	"github.com/MichaelKinsy/PiG/ai"
)

// installFailedStore prevents invalid connection configuration from silently
// activating old local credentials. The CLI repair path itself remains usable.
func installFailedStore(dir string, failure error) {
	profile, pathErr := filepath.Abs(dir)
	ai.CredentialStoreFactory = func(dir string) (ai.ApplicationCredentialStore, error) {
		path, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		if pathErr != nil || path == profile {
			return nil, failure
		}
		return ai.OpenLocalCredentialStore(dir)
	}
}

// installRemoteStore replaces credential discovery for exactly this profile.
// The factory stays installed on network failure, preventing a silent fallback
// to local refresh material. Configure once, before any runtimes are opened.
func installRemoteStore(client *Client) {
	ai.CredentialStoreFactory = func(dir string) (ai.ApplicationCredentialStore, error) {
		path, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		if path != client.dir {
			return ai.OpenLocalCredentialStore(dir)
		}
		return &remoteStore{client: client}, nil
	}
}
