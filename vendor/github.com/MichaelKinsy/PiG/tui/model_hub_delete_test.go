package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// TestModelHubDeleteRole exercises cancel-first confirmation, Escape, scope
// selection, persisted deletion reload and built-in confirmation through real keys.
func TestModelHubDeleteRole(t *testing.T) {
	h, config, chains := hubFixture()
	config.ModelRoles["review"] = "p/a"
	config.ModelTags = map[string]modelroles.Tag{"review": {Name: "Reviewer"}}
	config.CycleOrder = []string{"default", "review"}
	chains["review"] = []string{"p/b"}
	calls, scope := 0, ""
	h.cfg.ScopedConfig = func(string) modelroles.Config { return config.Clone() }
	h.cfg.DeleteRole = func(role, layer string) error {
		calls++
		scope = layer
		delete(config.ModelRoles, role)
		delete(config.ModelTags, role)
		delete(chains, role)
		config.CycleOrder = slices.DeleteFunc(config.CycleOrder, func(value string) bool { return value == role })
		return nil
	}
	h.result(nil)
	openRoles(h)
	for i, row := range h.rows {
		if row.kind == "role" && row.key == "review" {
			h.cursor = i
			break
		}
	}
	if !strings.Contains(h.footer(200), "D delete role") {
		t.Fatal("delete action undiscoverable")
	}
	h.HandleInput("D")
	if h.strip == nil || h.strip.kind != "deleteRole" || calls != 0 {
		t.Fatal("missing confirmation or premature deletion")
	}
	h.HandleInput("\r") // Cancel is selected by default.
	if h.strip != nil || calls != 0 {
		t.Fatal("Enter deleted without selecting the destructive action")
	}
	h.HandleInput("D")
	h.HandleInput("\x1b")
	if h.strip != nil || h.Done() || calls != 0 {
		t.Fatal("Escape did not cancel")
	}
	h.HandleInput("D")
	h.HandleInput("\x1b[C") // Project is the selected assignment's layer.
	h.HandleInput("\x1b[C") // Explicitly choose the other (global) layer.
	h.HandleInput("\r")
	if calls != 1 || scope != "global" || h.strip != nil {
		t.Fatal("delete scope", calls, scope)
	}
	for _, row := range h.rows {
		if row.key == "review" {
			t.Fatal("deleted role/fallback still in tree")
		}
	}
	h.cursor = 0 // Built-in default.
	h.HandleInput("D")
	if calls != 1 || h.strip == nil || h.strip.role != "default" {
		t.Fatal("built-in deletion did not ask for confirmation")
	}
	h.HandleInput("\x1b[C")
	h.HandleInput("\r")
	if calls != 2 || config.ModelRoles["default"] != "" {
		t.Fatal("built-in deletion callback was not invoked")
	}
}

// TestModelHubDeleteRoleFailure keeps the role present and shows persistence or
// reference errors rather than falsely reporting deletion. Clear stays separate.
func TestModelHubDeleteRoleFailure(t *testing.T) {
	h, config, _ := hubFixture()
	config.ModelRoles["review"] = "p/a"
	h.cfg.DeleteRole = func(string, string) error { return errors.New("disk full") }
	h.result(nil)
	openRoles(h)
	for i, row := range h.rows {
		if row.key == "review" {
			h.cursor = i
			break
		}
	}
	h.HandleInput("D")
	h.HandleInput("\x1b[C")
	h.HandleInput("\r")
	if config.ModelRoles["review"] == "" || h.status != "disk full" {
		t.Fatal("delete failure hidden")
	}
	h.HandleInput("d")
	if config.ModelRoles["review"] != "" {
		t.Fatal("lowercase d no longer clears assignment")
	}
}
