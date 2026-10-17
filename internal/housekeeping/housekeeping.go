// Package housekeeping keeps the pools healthy and the disk clean, by itself.
//
// It never runs `git gc`: gc prunes, and a pool must never lose an object. A pool is
// repacked with the objects nothing points at kept in a cruft pack that never expires, and
// only once it has piled up enough loose objects or packs to be worth it.
package housekeeping

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/metrics"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/strategy"
	"github.com/dooservice/gitd/internal/template"
)

// Keeper maintains the pools of one gitd, and sweeps the roots its workspaces are made in.
// A pool is repacked once it holds looseLimit loose objects or packLimit packs; temporary
// directories are looked for down to the configured sweep depth under the roots, where the
// deepest workspace is, and as deep as a repository's path goes under the pools.
type Keeper struct {
	pools         *pool.Pools
	templates     *template.Store
	overlay       *strategy.OverlayMaker // nil when workspaces are not made by overlay
	runner        *git.Runner
	roots         []string
	settings      config.Housekeeping
	metrics       *metrics.Metrics
	looseLimit    int
	packLimit     int
	poolCount     atomic.Int64
	poolBytes     atomic.Int64
	templateCount atomic.Int64
	templateBytes atomic.Int64
}

// TemplateTotals are how many templates there were and the disk they took, as of the last
// round.
func (keeper *Keeper) TemplateTotals() (int, int64) {
	return int(keeper.templateCount.Load()), keeper.templateBytes.Load()
}

// Totals are how many pools there were and the disk they took, as of the last round.
func (keeper *Keeper) Totals() (int, int64) {
	return int(keeper.poolCount.Load()), keeper.poolBytes.Load()
}

// New returns the keeper of pools and templates, whose workspaces are made under roots.
func New(
	pools *pool.Pools, templates *template.Store, overlay *strategy.OverlayMaker, runner *git.Runner, roots []string,
	settings config.Housekeeping, measured *metrics.Metrics,
) *Keeper {
	return &Keeper{
		pools:      pools,
		templates:  templates,
		overlay:    overlay,
		runner:     runner,
		roots:      roots,
		settings:   settings,
		metrics:    measured,
		looseLimit: 1024,
		packLimit:  8,
	}
}

// Run maintains every pool a minute after starting, so that it does not compete with the
// first calls after a start, and then every so often, until ctx ends.
func (keeper *Keeper) Run(ctx context.Context) {
	next := time.NewTimer(time.Minute)
	defer next.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-next.C:
		}
		keeper.round(ctx)
		next.Reset(keeper.settings.Every)
	}
}

// Remount mounts again the overlay workspaces that are not mounted — after a reboot, or
// after somebody unmounted one — and logs what it could not.
func (keeper *Keeper) Remount(ctx context.Context) int {
	if keeper.overlay == nil {
		return 0
	}
	remounted, failed := keeper.overlay.Remount(ctx)
	if remounted > 0 {
		slog.Info("overlay workspaces mounted again", "count", remounted)
	}
	for _, err := range failed {
		slog.Error("overlay workspace not mounted again", "error", err)
	}
	return remounted
}
