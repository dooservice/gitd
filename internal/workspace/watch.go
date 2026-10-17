package workspace

import (
	"context"
	"fmt"
	"hash/fnv"
	"io/fs"
	"path/filepath"
	"time"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/events"
	"github.com/dooservice/gitd/internal/strategy"
)

// layerLook is how often the agents' layers are looked at while someone watches.
const layerLook = 2 * time.Second

// UseEvents has workspaces tell what happens to them.
func (workspaces *Workspaces) UseEvents(bus *events.Bus) {
	workspaces.events = bus
}

// WatchLayers tells the watchers of an overlay workspace when something in its agent's
// layer changed, until ctx ends. The layers are looked at every couple of seconds, and only
// while someone watches: what an agent writes goes to its layer, which is small, so a look
// is a walk of what it changed, never of the tree. A workspace written whole has no layer
// to look at: its watchers hear of what gitd does to it, not of what its agent does.
func (workspaces *Workspaces) WatchLayers(ctx context.Context) {
	overlay, isOverlay := workspaces.maker.(*strategy.OverlayMaker)
	if !isOverlay || workspaces.events == nil {
		return
	}
	seen := map[string]uint64{}
	ticker := time.NewTicker(layerLook)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !workspaces.events.Watched() {
			clear(seen)
			continue
		}
		for _, record := range overlay.Records() {
			_, upper, _, ok := overlay.LayersOf(record.Workspace)
			if !ok {
				continue
			}
			current := fingerprint(upper)
			if before, known := seen[record.Workspace]; known && before != current {
				workspaces.events.Tell(&gitdv1.WatchResponse{Kind: gitdv1.WatchResponse_KIND_CHANGED, Path: record.Workspace})
			}
			seen[record.Workspace] = current
		}
	}
}

// fingerprint sums what a layer holds — every path, size and time it was changed — so that
// any write shows as a different sum.
func fingerprint(layer string) uint64 {
	sum := fnv.New64a()
	_ = filepath.WalkDir(layer, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			fmt.Fprintf(sum, "%s\x00%d\x00%d\x00", path, info.Size(), info.ModTime().UnixNano())
		}
		return nil
	})
	return sum.Sum64()
}
