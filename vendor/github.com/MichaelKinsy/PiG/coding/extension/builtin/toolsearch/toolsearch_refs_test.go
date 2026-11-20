package toolsearch

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
)

func TestCreateDocumentFollowsLocalSchemaRefs(t *testing.T) {
	tool := extension.ToolInfo{
		Name:        "mcp__archive__search",
		Description: "search archive records",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{"filter":{"$ref":"#/$defs/Filter"}},
			"$defs":{"Filter":{"type":"object","properties":{"includeArchived":{"type":"boolean","description":"Include archived records"}}}}
		}`),
	}
	document := CreateDocument(tool, nil)
	for _, expected := range []string{"includeArchived", "Include archived records"} {
		if !strings.Contains(document.Text, expected) {
			t.Errorf("search text %q does not include %q", document.Text, expected)
		}
	}
	matches := NewBm25Ranker().Rank("archived records", []Document{document}, DefaultLimit)
	if len(matches) != 1 || matches[0].Name != tool.Name {
		t.Fatalf("search matches = %#v, want %q", matches, tool.Name)
	}
}

func TestCreateDocumentFollowsEscapedAndArraySchemaRefs(t *testing.T) {
	tool := extension.ToolInfo{
		Name:        "mcp__archive__query",
		Description: "query archive",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{"filter":{"$ref":"#/$defs/a~1b/anyOf/0"}},
			"$defs":{"a/b":{"anyOf":[{"type":"object","properties":{"retentionWindow":{"type":"integer","description":"Retention window days"}}}]}}
		}`),
	}
	document := CreateDocument(tool, nil)
	if !strings.Contains(document.Text, "retentionWindow Retention window days") {
		t.Fatalf("search text does not include referenced schema: %q", document.Text)
	}
}

func TestCreateDocumentStopsRecursiveRefs(t *testing.T) {
	tool := extension.ToolInfo{
		Name:        "mcp__tree__inspect",
		Description: "inspect a tree",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{"root":{"$ref":"#/$defs/Node"}},
			"$defs":{"Node":{"type":"object","description":"tree node","properties":{"next":{"$ref":"#/$defs/Node"}}}}
		}`),
	}
	document := CreateDocument(tool, nil)
	if strings.Count(document.Text, "tree node") != 1 {
		t.Fatalf("recursive schema was not visited once: %q", document.Text)
	}
	matches := NewBm25Ranker().Rank("tree node", []Document{document}, DefaultLimit)
	if len(matches) != 1 || matches[0].Name != tool.Name {
		t.Fatalf("search matches = %#v, want %q", matches, tool.Name)
	}
}
