package workspace

import (
	"context"
	"path/filepath"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/events"
	"github.com/dooservice/gitd/internal/repo"
)

func TestWatchersHearWhatHappensToAWorkspace(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspaces(t, repository)
	bus := events.NewBus()
	workspaces.UseEvents(bus)
	watcher := bus.Watch(nil)
	path := made(t, workspaces, roots, "agent", repository)
	_, _ = workspaces.Publish(context.Background(), &gitdv1.PublishRequest{Path: path, Branch: "agents/x"})
	if err := workspaces.Remove(context.Background(), &gitdv1.RemoveRequest{Path: path}); err != nil {
		t.Fatal(err)
	}
	var kinds []gitdv1.WatchResponse_Kind
	for len(watcher.Events) > 0 {
		event := <-watcher.Events
		if event.GetPath() != path {
			t.Errorf("an event of %s", event.GetPath())
		}
		kinds = append(kinds, event.GetKind())
	}
	want := []gitdv1.WatchResponse_Kind{
		gitdv1.WatchResponse_KIND_MADE, gitdv1.WatchResponse_KIND_REFUSED, gitdv1.WatchResponse_KIND_REMOVED,
	}
	if len(kinds) != len(want) || kinds[0] != want[0] || kinds[1] != want[1] || kinds[2] != want[2] {
		t.Errorf("heard %v; want %v", kinds, want)
	}
}

func TestALayersFingerprintChangesWithWhatIsWrittenThere(t *testing.T) {
	layer := t.TempDir()
	write(t, filepath.Join(layer, "src/app.go"), "a")
	before := fingerprint(layer)
	if fingerprint(layer) != before {
		t.Fatal("the same layer gave two fingerprints")
	}
	write(t, filepath.Join(layer, "src/app.go"), "ab")
	if fingerprint(layer) == before {
		t.Error("a write left the fingerprint as it was")
	}
}
