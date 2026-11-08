package ai

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/MichaelKinsy/PiG/internal/scalarjson"
)

// ASCII case folding preserves byte length. Grouping the fixed modeled names
// avoids allocating a lowercase copy of every opaque key on every model read.
var compatNamesByLength = sync.OnceValue(func() map[int][]string {
	names := map[int][]string{}
	for name := range openAICompatMembers() {
		names[len(name)] = append(names[len(name)], name)
	}
	return names
})

// cloneScalarCompat preserves scalar fields and opaque extension JSON without
// serializing every modeled field. Complex typed fields retain the existing
// whole-object fallback. Nonliteral Extra values still pass through marshaling
// so invalid JSON and whitespace/HTML normalization match the serialized copy.
// Empty raw values and colliding/non-ASCII keys use the fallback because
// whole-object validation, JSON field matching and Unicode
// repair can change modeled fields or key spelling.
func cloneScalarCompat(in *ModelCompat) (*ModelCompat, bool) {
	out := *in
	out.Extra = nil
	if !scalarjson.CloneFields(reflect.ValueOf(&out).Elem()) {
		return nil, false
	}
	if len(in.Extra) > 0 {
		out.Extra = make(map[string]json.RawMessage, len(in.Extra))
		for key, value := range in.Extra {
			// AppendMembers writes raw bytes verbatim; nil/empty is invalid there,
			// unlike marshaling a standalone nil RawMessage (which emits null).
			if len(value) == 0 {
				return nil, false
			}
			for i := 0; i < len(key); i++ {
				if key[i] >= utf8.RuneSelf {
					return nil, false
				}
			}
			for _, name := range compatNamesByLength()[len(key)] {
				if strings.EqualFold(key, name) {
					return nil, false
				}
			}
			// These three complete JSON literals need only whitespace removal and a
			// detached copy. All strings, numbers and containers retain the serializer.
			trimmed := bytes.TrimSpace(value)
			switch string(trimmed) {
			case "true", "false", "null":
				out.Extra[key] = bytes.Clone(trimmed)
			default:
				encoded, err := json.Marshal(value)
				if err != nil {
					return nil, false
				}
				out.Extra[key] = encoded
			}
		}
	}
	return &out, true
}
