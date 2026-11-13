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
