package tui

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"slices"
	"testing"

	"github.com/MichaelKinsy/PiG/extensions/sdk"
	sdktui "github.com/MichaelKinsy/PiG/extensions/sdk/tui"
)

func TestSDKUsesSharedTUIKeybindings(t *testing.T) {
	definitions := TUIKeybindingDefinitions()
	if len(definitions) != len(sdktui.TUIKeybindings) {
		t.Fatalf("SDK has %d definitions, runtime has %d", len(sdktui.TUIKeybindings), len(definitions))
	}
	for action, definition := range definitions {
		got, ok := sdktui.TUIKeybindings[action]
		if !ok || !slices.Equal(got.DefaultKeys, definition.DefaultKeys) || got.Description != definition.Description {
			t.Errorf("SDK definition for %s = %+v, want %+v", action, got, definition)
		}
	}
}

func TestEmbeddedSDKUsesSharedKeybindingSource(t *testing.T) {
	for _, name := range []string{"keybindings.go", "alt_screen_keybindings.go"} {
		t.Run(name, func(t *testing.T) {
			path := "tui/internal/engine/" + name
			packed, err := sdk.Source.ReadFile(".gz/" + path + ".gz")
			if err != nil {
				t.Fatal(err)
			}
			reader, err := gzip.NewReader(bytes.NewReader(packed))
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			got, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile("../extensions/sdk/" + path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("embedded SDK keybinding source differs from maintained source")
			}
		})
	}
}
