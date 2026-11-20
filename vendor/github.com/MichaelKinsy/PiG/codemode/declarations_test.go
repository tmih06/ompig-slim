package codemode

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSchemaToTypeLocalRefEdges(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		want   string
	}{
		{
			name:   "empty pointer segment",
			schema: `{"$ref":"#/properties/","properties":{"":{"type":"string"}}}`,
			want:   "string",
		},
		{
			name:   "escaped key and array index",
			schema: `{"$ref":"#/$defs/a~1b/anyOf/0","$defs":{"a/b":{"anyOf":[{"type":"integer"}]}}}`,
			want:   "number",
		},
		{
			name:   "URI-escaped key",
			schema: `{"$ref":"#/$defs/a%20b","$defs":{"a b":{"type":"boolean"}}}`,
			want:   "boolean",
		},
		{
			name:   "malformed percent escape",
			schema: `{"$ref":"#/$defs/%E0"}`,
			want:   "unknown",
		},
		{
			name:   "unresolved reference",
			schema: `{"$ref":"#/$defs/missing"}`,
			want:   "unknown",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := SchemaToType(json.RawMessage(test.schema), SchemaToTypeOptions{})
			if err != nil {
				t.Fatalf("SchemaToType returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("SchemaToType = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderDeclarationsMalformedLocalRef(t *testing.T) {
	declarations, err := RenderDeclarations(RenderDeclarationsOptions{Tools: []Tool{{
		Name:        "search",
		Description: "search documents",
		InputSchema: json.RawMessage(`{"$ref":"#/$defs/%E0"}`),
	}}})
	if err != nil {
		t.Fatalf("RenderDeclarations returned error: %v", err)
	}
	if !strings.Contains(declarations, "search(args: unknown)") {
		t.Fatalf("malformed reference was not rendered as unknown: %s", declarations)
	}
}
