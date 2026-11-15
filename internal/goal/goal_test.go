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
	if restart, err := tracker.Resume(); err != nil {
		t.Fatal(err)
	} else if !restart {
		t.Fatal("resuming a paused goal must restart its objective")
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

// TestResumeReportsWhetherToRestart pins the resume contract the goal command
// depends on: a paused goal restarts once, and repeating resume starts nothing.
func TestResumeReportsWhetherToRestart(t *testing.T) {
	tracker := New()
	if err := tracker.Create("finish the work", 0); err != nil {
		t.Fatal(err)
	}
	if restart, err := tracker.Resume(); err != nil || restart {
		t.Fatalf("resume of an active goal = (%v, %v), want (false, nil)", restart, err)
	}
	if err := tracker.Pause(); err != nil {
		t.Fatal(err)
	}
	if restart, err := tracker.Resume(); err != nil || !restart {
		t.Fatalf("resume of a paused goal = (%v, %v), want (true, nil)", restart, err)
	}
	if restart, err := tracker.Resume(); err != nil || restart {
		t.Fatalf("second resume = (%v, %v), want (false, nil)", restart, err)
	}
	if state := tracker.Get(); state == nil || state.Status != StatusActive {
		t.Fatalf("resumed state = %+v", state)
	}
}
