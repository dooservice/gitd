package events

import (
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func TestAWatcherGetsTheEventsOfItsWorkspacesOnly(t *testing.T) {
	bus := NewBus()
	every, one := bus.Watch(nil), bus.Watch([]string{"/w/agent-1"})
	bus.Tell(&gitdv1.WatchResponse{Kind: gitdv1.WatchResponse_KIND_MADE, Path: "/w/agent-1"})
	bus.Tell(&gitdv1.WatchResponse{Kind: gitdv1.WatchResponse_KIND_MADE, Path: "/w/agent-2"})
	if len(every.Events) != 2 || len(one.Events) != 1 {
		t.Errorf("every got %d, one got %d", len(every.Events), len(one.Events))
	}
	if event := <-one.Events; event.GetPath() != "/w/agent-1" || event.GetTime() == nil {
		t.Errorf("event = %v", event)
	}
	bus.Stop(every)
	bus.Stop(one)
	if bus.Watched() {
		t.Error("watched after every watch stopped")
	}
}

func TestASlowWatcherIsDroppedNotWaitedFor(t *testing.T) {
	bus := NewBus()
	slow := bus.Watch(nil)
	for range queued + 1 {
		bus.Tell(&gitdv1.WatchResponse{Kind: gitdv1.WatchResponse_KIND_CHANGED, Path: "/w/agent-1"})
	}
	received := 0
	for range slow.Events {
		received++
	}
	if received != queued || bus.Watched() {
		t.Errorf("received %d before being dropped; still watched %v", received, bus.Watched())
	}
}
