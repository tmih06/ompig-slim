//go:build !pig_strip_node_extensions

package subprocess

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"path"
	"regexp"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/internal/noderuntimepack"
	"github.com/MichaelKinsy/PiG/extensions/sdk"
	pysdk "github.com/MichaelKinsy/PiG/extensions/sdk-py"
	rssdk "github.com/MichaelKinsy/PiG/extensions/sdk-rs"
)

// retiredModelScopeAPI matches feature identifiers, not provider account model
// allowlists or global/project settings scopes, which have different semantics.
var retiredModelScopeAPI = regexp.MustCompile(`\b(?:ScopedModel|ScopedModels|scopedModels|scopedModelItems|_scopedModels|scoped_models|get_scoped_models|ModelScopeRuntime|resolveModelScope(?:FromModels|WithDiagnostics)?|getScopedModels|setScopedModels|getEnabledModels|setEnabledModels|enabledModels|get_enabled_models|set_enabled_models|isScoped)\b`)

// TestBundledSDKsAreRoleOnly guards the actual staged source archives as well
// as the compiled Go surface: removing host handlers must not leave dormant
// implementations in the Go, Python or Rust SDKs an installed binary ships.
func TestBundledSDKsAreRoleOnly(t *testing.T) {
	for _, bundle := range []struct {
		name  string
		files fs.FS
	}{{"go", sdk.Source}, {"python", pysdk.Source}, {"rust", rssdk.Source}} {
		t.Run(bundle.name, func(t *testing.T) {
			err := fs.WalkDir(bundle.files, ".", func(name string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				encoded, err := fs.ReadFile(bundle.files, name)
				if err != nil {
					return err
				}
				reader, err := gzip.NewReader(bytes.NewReader(encoded))
				if err != nil {
					return err
				}
				source, err := io.ReadAll(reader)
				_ = reader.Close()
				if err != nil {
					return err
				}
				if match := retiredModelScopeAPI.Find(source); match != nil {
					t.Errorf("retired SDK API %s remains in %s", match, name)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestBundledNodeModelsAreRoleOnly decodes the shipped archive with its canonical
// codec. Upstream changelogs are historical; only executable JS modules are
// checked. Pack extraction is concurrent; regexp and testing errors are safe.
func TestBundledNodeModelsAreRoleOnly(t *testing.T) {
	err := noderuntimepack.Extract(nodeRuntimeArchive, func(name string, source []byte) error {
		if ext := path.Ext(name); ext == ".js" || ext == ".mjs" {
			if match := retiredModelScopeAPI.Find(source); match != nil {
				t.Errorf("retired Node API %s remains in %s", match, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
