package tui

import (
	"context"
	"testing"
)

// keywordProvider pairs a keyword-tagged command with plain and skill entries.
func keywordProvider() *SlashOnlyProvider {
	return NewSlashOnlyProvider([]SlashCommand{
		{Name: "model"},
		{Name: "new", Description: "Start a new session", Keywords: []string{"clear", "fresh"}},
		{Name: "skill:review"},
	})
}

// TestSlashKeywordMatchingAndLabel observes name completion: a purpose word
// matches a command through its keywords, the row shows them as a
// parenthesized tag, and accepting still inserts only the command name.
func TestSlashKeywordMatchingAndLabel(t *testing.T) {
	p := keywordProvider()
	for _, query := range []string{"/clea", "/fresh", "/clear"} {
		result := p.GetSuggestions(context.Background(), []string{query}, 0, len(query), AutocompleteSuggestionOptions{})
		if result == nil || len(result.Items) == 0 || result.Items[0].Value != "new" {
			t.Fatalf("%q did not rank /new first: %+v", query, result)
		}
		if result.Items[0].Label != "new (clear, fresh)" {
			t.Fatalf("%q label = %q", query, result.Items[0].Label)
		}
		if result.Items[0].Description != "Start a new session" {
			t.Fatalf("%q description = %q", query, result.Items[0].Description)
		}
		lines, row, col := p.ApplyCompletion([]string{query}, 0, len(query), result.Items[0], result.Prefix)
		if lines[row] != "/new " || row != 0 || col != len("/new ") {
			t.Fatalf("%q accepted as %q @%d,%d", query, lines[row], row, col)
		}
	}
	if result := p.GetSuggestions(context.Background(), []string{"/zzz"}, 0, len("/zzz"), AutocompleteSuggestionOptions{}); result != nil {
		t.Fatalf("unmatched query produced suggestions: %+v", result)
	}
}

// TestSlashKeywordRowsKeepPlainLabels observes the unfiltered popup: only
// keyword-carrying commands show a tag; plain and skill rows stay bare.
func TestSlashKeywordRowsKeepPlainLabels(t *testing.T) {
	result := keywordProvider().GetSuggestions(context.Background(), []string{"/"}, 0, len("/"), AutocompleteSuggestionOptions{})
	if result == nil {
		t.Fatal("empty query produced no command list")
	}
	want := map[string]string{"model": "model", "new": "new (clear, fresh)", "skill:review": "skill:review"}
	for _, item := range result.Items {
		if item.Label != want[item.Value] {
			t.Fatalf("%q label = %q", item.Value, item.Label)
		}
	}
}
