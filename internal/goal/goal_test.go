package goal

import "testing"

func TestLifecycleKeepsObjectiveUntilDropped(t *testing.T) {
	tracker := New()
	if err := tracker.Create("Ship the release", 100); err != nil {
		t.Fatal(err)
	}
	state := tracker.Get()
	if state == nil || state.Objective != "Ship the release" || state.Status != StatusActive {
		t.Fatalf("created state = %+v", state)
	}
	if err := tracker.Pause(); err != nil {
		t.Fatal(err)
	}
	if state = tracker.Get(); state == nil || state.Status != StatusPaused {
		t.Fatalf("paused state = %+v", state)
	}
	if err := tracker.Resume(); err != nil {
		t.Fatal(err)
	}
	if state = tracker.Get(); state == nil || state.Status != StatusActive {
		t.Fatalf("resumed state = %+v", state)
	}
	if err := tracker.Complete(); err != nil {
		t.Fatal(err)
	}
	if state = tracker.Get(); state == nil || state.Status != StatusComplete {
		t.Fatalf("completed state = %+v", state)
	}
	if err := tracker.Drop(); err != nil {
		t.Fatal(err)
	}
	if state = tracker.Get(); state != nil {
		t.Fatalf("dropped state = %+v, want nil", state)
	}
}

func TestPokeCountsGoalContinuations(t *testing.T) {
	tracker := New()
	if err := tracker.Create("finish the work", 0); err != nil {
		t.Fatal(err)
	}
	if got := tracker.Poke(); got != 1 {
		t.Fatalf("first poke = %d, want 1", got)
	}
	if got := tracker.Get().Pokes; got != 1 {
		t.Fatalf("stored pokes = %d, want 1", got)
	}
}

func TestCreateRejectsInvalidGoal(t *testing.T) {
	tracker := New()
	if err := tracker.Create(" ", 0); err == nil {
		t.Fatal("empty objective was accepted")
	}
	if err := tracker.Create("work", -1); err == nil {
		t.Fatal("negative budget was accepted")
	}
}

// TestRepeatedBlockerPausesAtThreshold pins the whole policy on the tracker, the
// one place that can enforce it.
func TestRepeatedBlockerPausesAtThreshold(t *testing.T) {
	tracker := New()
	if err := tracker.Create("finish the work", 0); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < BlockerThreshold; i++ {
		state, err := tracker.Blocked("needs the staging API token")
		if err != nil {
			t.Fatalf("report %d: %v", i, err)
		}
		if state.BlockerCount != i || state.Status != StatusActive {
			t.Fatalf("after report %d: %+v, want count %d and an active goal", i, state, i)
		}
	}
	state, err := tracker.Blocked("needs the staging API token")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusPaused {
		t.Fatalf("status after %d reports = %s, want paused", BlockerThreshold, state.Status)
	}
	if state.BlockerCount != BlockerThreshold {
		t.Fatalf("count = %d, want %d", state.BlockerCount, BlockerThreshold)
	}
	if _, err := tracker.Blocked("needs the staging API token"); err == nil {
		t.Fatal("a paused goal accepted another blocker report")
	}
	if err := tracker.Resume(); err != nil {
		t.Fatal(err)
	}
	if state = tracker.Get(); state.BlockerCount != 0 || state.Blocker != "needs the staging API token" {
		t.Fatalf("resumed state = %+v, want the streak cleared and the explanation kept", state)
	}
}

// TestBlockerIdentityIgnoresCaseAndWhitespace pins what "the same blocker" means.
func TestBlockerIdentityIgnoresCaseAndWhitespace(t *testing.T) {
	tracker := New()
	if err := tracker.Create("finish the work", 0); err != nil {
		t.Fatal(err)
	}
	reports := []string{"Needs the API token", "  needs   the API\ttoken ", "NEEDS THE API TOKEN"}
	for i, reason := range reports {
		state, err := tracker.Blocked(reason)
		if err != nil {
			t.Fatal(err)
		}
		if state.BlockerCount != i+1 {
			t.Fatalf("%q counted %d, want %d", reason, state.BlockerCount, i+1)
		}
	}
	if state := tracker.Get(); state.Status != StatusPaused {
		t.Fatalf("status = %s, want paused", state.Status)
	}
}

func TestProgressAndDifferentBlockerResetTheStreak(t *testing.T) {
	tracker := New()
	if err := tracker.Create("finish the work", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := tracker.Blocked("needs the API token"); err != nil {
		t.Fatal(err)
	}
	if _, err := tracker.Blocked("needs the API token"); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Progress(); err != nil {
		t.Fatal(err)
	}
	if state := tracker.Get(); state.BlockerCount != 0 || state.Blocker != "" {
		t.Fatalf("progress left %+v, want an empty streak", state)
	}
	if _, err := tracker.Blocked("needs the API token"); err != nil {
		t.Fatal(err)
	}
	// A different blocker starts its own count instead of continuing the first.
	state, err := tracker.Blocked("needs a decision on the migration order")
	if err != nil {
		t.Fatal(err)
	}
	if state.BlockerCount != 1 || state.Status != StatusActive {
		t.Fatalf("state = %+v, want a fresh count of 1", state)
	}
}

func TestBlockedRejectsEmptyReason(t *testing.T) {
	tracker := New()
	if err := tracker.Create("finish the work", 0); err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"", "   ", "\n\t"} {
		if _, err := tracker.Blocked(reason); err == nil {
			t.Fatalf("blank reason %q was accepted", reason)
		}
	}
}

// TestLateReportsCannotRestartASettledGoal keeps a queued blocker report or a
// stray progress report from reviving work a human stopped.
func TestLateReportsCannotRestartASettledGoal(t *testing.T) {
	for _, settle := range []struct {
		name string
		do   func(*Tracker) error
	}{
		{"paused", (*Tracker).Pause},
		{"complete", (*Tracker).Complete},
	} {
		t.Run(settle.name, func(t *testing.T) {
			tracker := New()
			if err := tracker.Create("finish the work", 0); err != nil {
				t.Fatal(err)
			}
			if err := settle.do(tracker); err != nil {
				t.Fatal(err)
			}
			if _, err := tracker.Blocked("needs the API token"); err == nil {
				t.Fatal("a settled goal accepted a blocker report")
			}
			if err := tracker.Progress(); err == nil {
				t.Fatal("a settled goal accepted a progress report")
			}
			if state := tracker.Get(); state.Status != mustStatus(settle.name) {
				t.Fatalf("status = %s, want %s", state.Status, mustStatus(settle.name))
			}
		})
	}
}

// TestNewGoalDoesNotInheritTheStreak keeps a dropped goal's blocker out of the next one.
func TestNewGoalDoesNotInheritTheStreak(t *testing.T) {
	tracker := New()
	if err := tracker.Create("first objective", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := tracker.Blocked("needs the API token"); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Drop(); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Create("second objective", 0); err != nil {
		t.Fatal(err)
	}
	if state := tracker.Get(); state.BlockerCount != 0 || state.Blocker != "" {
		t.Fatalf("new goal = %+v, want no inherited blocker", state)
	}
}

func mustStatus(name string) Status {
	if name == "complete" {
		return StatusComplete
	}
	return StatusPaused
}
