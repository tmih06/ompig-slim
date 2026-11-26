package codingagent

import "testing"

func TestHasQueuedRegistrationRefresh(t *testing.T) {
	registry := NewModelRegistry(t.TempDir())
	defer registry.CloseModelTasks()
	if registry.HasQueuedRegistrationRefresh() {
		t.Fatal("new registry reported a queued registration refresh")
	}
	registry.deferRegistrationRefresh("fixture")
	if !registry.HasQueuedRegistrationRefresh() {
		t.Fatal("queued provider registration was not visible to startup")
	}
	release := registry.HoldRegistrationRefresh()
	defer release()
	covered := registry.coverRegistrationRefresh([]string{"fixture"})
	if len(covered) != 1 || registry.HasQueuedRegistrationRefresh() {
		t.Fatal("covered provider registration remained queued")
	}
	registry.uncoverRegistrationRefresh(covered)
	if !registry.HasQueuedRegistrationRefresh() {
		t.Fatal("uncovered provider registration did not return to the queue")
	}
}
