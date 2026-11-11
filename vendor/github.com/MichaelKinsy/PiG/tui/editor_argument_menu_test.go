package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// argumentMenuProvider is a deterministic command with described argument rows;
// no filesystem, extension process, or runtime session is needed by these tests.
func argumentMenuProvider() *SlashOnlyProvider {
	return NewSlashOnlyProvider([]SlashCommand{{
		Name: "example",
		GetArgumentCompletions: func(prefix string) []AutocompleteItem {
			if prefix != "" {
				return nil
			}
			return []AutocompleteItem{{Value: "on", Label: "on", Description: "Enable example"}}
		},
	}})
}

// TestArgumentMenuAfterNativeCheckpoint verifies the production owner-loop path,
// including forced single-result acceptance, without starting a query worker.
func TestArgumentMenuAfterNativeCheckpoint(t *testing.T) {
	for _, forced := range []bool{false, true} {
		e := NewEditor()
		e.SetAutocomplete(argumentMenuProvider())
		e.SetAsyncApply(func(func()) {}) // The test explicitly flushes each read.
		e.SetAutocompleteTaskOwner(context.Background(), func(func()) {
			t.Fatal("synchronous suggestions should not start a worker")
		}, func(err error) { t.Fatal(err) })
		e.SetText("/exa")
		if forced {
			e.requestNativeAutocomplete(true, true)
		} else {
			e.RefreshAutocomplete()
		}
		e.FlushAutocomplete()
		if !forced {
			e.HandleInput("\t")
		}
		e.FlushAutocomplete()
		if e.Text() != "/example " || !e.AutocompleteOpen() || e.autocompletePrefix != "" {
			t.Fatalf("forced=%t: text=%q prefix=%q open=%t", forced, e.Text(), e.autocompletePrefix, e.AutocompleteOpen())
		}
		if e.autocompleteItems[0].Description != "Enable example" {
			t.Fatal("argument purpose was lost")
		}
	}
}

// TestSlashCommandContextDoesNotFallThroughToPathCompletion keeps an
// unrecognized command-shaped line out of the filesystem completion branch.
func TestSlashCommandContextDoesNotFallThroughToPathCompletion(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	provider := NewCombinedProvider([]SlashCommand{{Name: "known"}}, dir, "")
	if got := provider.GetSuggestions(context.Background(), []string{"/unknown "}, 0, len("/unknown "), AutocompleteSuggestionOptions{}); got != nil {
		t.Fatalf("slash command context produced path suggestions: %+v", got)
	}
}

// TestAsyncNameCompletionRequestsArguments checks that an extension wrapper's
// completion schedules an argument query on Tab but not on Enter-to-submit.
func TestAsyncNameCompletionRequestsArguments(t *testing.T) {
	for _, submit := range []bool{false, true} {
		e := NewEditor()
		base := argumentMenuProvider()
		e.SetAsyncApply(func(func()) {})
		e.SetAsyncAutocomplete(&AsyncAutocompleteProvider{
			ApplyCompletion: func(_ context.Context, lines []string, row, col int, item AutocompleteItem, prefix string) ([]string, int, int, error) {
				lines, row, col = base.ApplyCompletion(lines, row, col, item, prefix)
				return lines, row, col, nil
			},
		}, context.Background(), func(func()) {}, func(work AutocompleteWork) {
			apply, err := work(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			apply()
		}, func(err error) { t.Fatal(err) })
		e.SetText("/exa")
		e.publishAutocomplete(false, false, e.cursor, base.GetSuggestions(context.Background(), e.lines, 0, 4, AutocompleteSuggestionOptions{}), nil)
		var done func(bool)
		if submit {
			done = func(got bool) {
				if !got {
					t.Fatal("Enter did not request submission")
				}
			}
		}
		e.acceptAsyncAutocomplete(done)
		if e.Text() != "/example " || (e.nativeAutocomplete != nil) == submit {
			t.Fatalf("submit=%t: text=%q pending=%t", submit, e.Text(), e.nativeAutocomplete != nil)
		}
	}
}
