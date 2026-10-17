// Package events carries what happens to workspaces to whoever watches: a workspace made,
// changed, published, refused or removed.
package events

import (
	"slices"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// queued is how many events a watcher may fall behind by before it is dropped: gitd never
// waits for a slow reader.
const queued = 256

// Bus hands each event to every watcher of its workspace.
type Bus struct {
	guard    sync.Mutex
	watchers map[*Watcher]struct{}
}

// Watcher is one watch: its events, and the workspaces it watches (every one when none).
type Watcher struct {
	Events chan *gitdv1.WatchResponse
	paths  []string
	closed bool
}

// NewBus returns a bus no one watches yet.
func NewBus() *Bus {
	return &Bus{watchers: map[*Watcher]struct{}{}}
}

// Watch starts a watch of the workspaces at paths, or of every one.
func (bus *Bus) Watch(paths []string) *Watcher {
	watcher := &Watcher{Events: make(chan *gitdv1.WatchResponse, queued), paths: paths}
	bus.guard.Lock()
	defer bus.guard.Unlock()
	bus.watchers[watcher] = struct{}{}
	return watcher
}

// Stop ends a watch.
func (bus *Bus) Stop(watcher *Watcher) {
	bus.guard.Lock()
	defer bus.guard.Unlock()
	bus.drop(watcher)
}

// Watched says whether anyone watches, so that what costs to notice is noticed only then.
func (bus *Bus) Watched() bool {
	bus.guard.Lock()
	defer bus.guard.Unlock()
	return len(bus.watchers) > 0
}

// Tell hands an event to every watcher of its workspace, stamped with the time now. A
// watcher too far behind is dropped, its events closed.
func (bus *Bus) Tell(event *gitdv1.WatchResponse) {
	if bus == nil {
		return
	}
	event.Time = timestamppb.New(time.Now())
	bus.guard.Lock()
	defer bus.guard.Unlock()
	for watcher := range bus.watchers {
		if len(watcher.paths) > 0 && !slices.Contains(watcher.paths, event.GetPath()) {
			continue
		}
		select {
		case watcher.Events <- event:
		default:
			bus.drop(watcher)
		}
	}
}

func (bus *Bus) drop(watcher *Watcher) {
	if _, watching := bus.watchers[watcher]; watching {
		delete(bus.watchers, watcher)
		close(watcher.Events)
	}
}
