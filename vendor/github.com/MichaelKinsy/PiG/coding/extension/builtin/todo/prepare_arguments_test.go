package todo

import (
	"encoding/json"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestPrepareArgumentsRepairsOmittedOp pins the repair the schema cannot do for
// itself.
//
// `params` requires `op`, and the harness validates a call's arguments before
// its tool runs. A repair written in execute therefore never runs for the call it
// exists to fix: the op-less payload dies as `op: missing` first. This hook runs
// ahead of validation, so every case below has to leave the validator a payload
// it accepts and execute an op it can act on.
//
// The rejected cases matter as much as the repaired ones. A repair that guesses
// wrong here mutates a list the model did not mean to touch, so a payload whose
// op genuinely cannot be told apart must still reach the model as a validation
// error it can correct.
func TestPrepareArgumentsRepairsOmittedOp(t *testing.T) {
	validated := func(t *testing.T, raw string) map[string]any {
		t.Helper()
		prepared, err := prepareArguments(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("prepareArguments(%s): %v", raw, err)
		}
		out, err := ai.ValidateToolArgumentsJSON(ToolName, json.RawMessage(params), prepared)
		if err != nil {
			t.Fatalf("validation rejected the repaired %s: %v", raw, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(out, &decoded); err != nil {
			t.Fatalf("decoding the validated arguments: %v", err)
		}
		return decoded
	}

	t.Run("a list is init", func(t *testing.T) {
		got := validated(t, `{"list":[{"phase":"P","items":["a"]}]}`)
		if got["op"] != "init" {
			t.Fatalf("op = %v, want init", got["op"])
		}
	})

	t.Run("items beside a phase is append", func(t *testing.T) {
		got := validated(t, `{"phase":"P","items":["a"]}`)
		if got["op"] != "append" {
			t.Fatalf("op = %v, want append", got["op"])
		}
	})

	t.Run("an empty call is view", func(t *testing.T) {
		// The observed failure: a model that calls `todo` with no arguments is
		// asking to see the list. view is the only op that takes no argument, and
		// it is read-only, so the guess cannot damage the list.
		got := validated(t, `{}`)
		if got["op"] != "view" {
			t.Fatalf("op = %v, want view", got["op"])
		}
	})

	t.Run("a null op is the same omission", func(t *testing.T) {
		got := validated(t, `{"op":null}`)
		if got["op"] != "view" {
			t.Fatalf("op = %v, want view", got["op"])
		}
	})

	t.Run("a named op is left alone", func(t *testing.T) {
		prepared, err := prepareArguments(json.RawMessage(`{"op":"done","task":"a"}`))
		if err != nil {
			t.Fatalf("prepareArguments: %v", err)
		}
		if string(prepared) != `{"op":"done","task":"a"}` {
			t.Fatalf("prepareArguments rewrote a named op: %s", prepared)
		}
	})

	t.Run("an unknown op is left for validation to reject", func(t *testing.T) {
		if _, err := prepareArguments(json.RawMessage(`{"op":"frobnicate"}`)); err != nil {
			t.Fatalf("prepareArguments: %v", err)
		}
		prepared, _ := prepareArguments(json.RawMessage(`{"op":"frobnicate"}`))
		if _, err := ai.ValidateToolArgumentsJSON(ToolName, json.RawMessage(params), prepared); err == nil {
			t.Fatal("an unknown op must still fail validation")
		}
	})

	t.Run("a bare items list is a flat init", func(t *testing.T) {
		// append rejects a missing phase, so init is the only op that can succeed.
		got := validated(t, `{"items":["a"]}`)
		if got["op"] != "init" {
			t.Fatalf("op = %v, want init", got["op"])
		}
	})

	t.Run("an empty items list is left ambiguous", func(t *testing.T) {
		// Nothing to init and nothing to append: no op can succeed, so the model is
		// told about the missing op rather than being handed one that fails later.
		prepared, err := prepareArguments(json.RawMessage(`{"items":[]}`))
		if err != nil {
			t.Fatalf("prepareArguments: %v", err)
		}
		if _, err := ai.ValidateToolArgumentsJSON(ToolName, json.RawMessage(params), prepared); err == nil {
			t.Fatal("an ambiguous payload must still report the missing op")
		}
	})

	t.Run("arguments that are not an object pass through", func(t *testing.T) {
		prepared, err := prepareArguments(json.RawMessage(`"nope"`))
		if err != nil {
			t.Fatalf("prepareArguments: %v", err)
		}
		if string(prepared) != `"nope"` {
			t.Fatalf("prepareArguments rewrote a non-object: %s", prepared)
		}
	})
}

// TestPreparedOpSurvivesExecution drives a repaired payload all the way through
// execute, so the repair is proven to reach the tool and not just the validator.
// A hook that satisfies the schema while execute still sees no op would trade one
// opaque error for another.
func TestPreparedOpSurvivesExecution(t *testing.T) {
	prepared, err := prepareArguments(json.RawMessage(`{"list":[{"phase":"P","items":["a","b"]}]}`))
	if err != nil {
		t.Fatalf("prepareArguments: %v", err)
	}
	validated, err := ai.ValidateToolArgumentsJSON(ToolName, json.RawMessage(params), prepared)
	if err != nil {
		t.Fatalf("validation: %v", err)
	}
	result, err := execute(t.Context(), "", validated, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("execute rejected the repaired call: %s", result.Text())
	}
	phases := result.Details.(Details).Phases
	if len(phases) != 1 || len(phases[0].Tasks) != 2 {
		t.Fatalf("execute did not apply the init: %+v", phases)
	}
}

// TestEmptyCallReadsTheList pins that an argument-less call reports the list
// rather than rewriting it. view is exempt from normalization, so the phases come
// back exactly as they were stored.
func TestEmptyCallReadsTheList(t *testing.T) {
	prepared, err := prepareArguments(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("prepareArguments: %v", err)
	}
	validated, err := ai.ValidateToolArgumentsJSON(ToolName, json.RawMessage(params), prepared)
	if err != nil {
		t.Fatalf("validation: %v", err)
	}
	result, err := execute(t.Context(), "", validated, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("a bare todo call must not fail: %s", result.Text())
	}
	if details := result.Details.(Details); details.Op != OpView {
		t.Fatalf("op = %q, want a view", details.Op)
	}
}
