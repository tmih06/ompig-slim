package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateToolArgumentsCoercion(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer"},"labels":{"type":"array","items":{"type":"string"}}}}`)
	got, err := ValidateToolArgumentsJSON("fixture", schema, json.RawMessage(`{"count":"42","labels":"archive"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"count":42,"labels":["archive"]}` {
		t.Fatalf("coerced arguments = %s", got)
	}
}

func TestValidateToolArgumentsOmitsNullOptionalProperty(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer"}}}`)
	got, err := ValidateToolArgumentsJSON("fixture", schema, json.RawMessage(`{"count":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{}` {
		t.Fatalf("validated arguments = %s, want {}", got)
	}
}

func TestValidateToolArgumentsRejectsDanglingRefAtItsValue(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"item":{"$ref":"#/$defs/Missing"}}}`)
	if _, err := ValidateToolArgumentsJSON("fixture", schema, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("absent optional reference rejected: %v", err)
	}
	_, err := ValidateToolArgumentsJSON("fixture", schema, json.RawMessage(`{"item":{}}`))
	if err == nil || !strings.Contains(err.Error(), "item: unresolved reference #/$defs/Missing") {
		t.Fatalf("dangling reference error = %v", err)
	}
}
