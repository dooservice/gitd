// Package housekeeping keeps the pools healthy and the disk clean, by itself.
//
// It never runs `git gc`: gc prunes, and a pool must never lose an object. A pool is
// repacked with the objects nothing points at kept in a cruft pack that never expires, and
// only once it has piled up enough loose objects or packs to be worth it.
package housekeeping

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/metrics"
	"github.com/dooservice/gitd/internal/pool"
)

// Keeper maintains the pools of one gitd, and sweeps the roots its workspaces are made in.
// A pool is repacked once it holds looseLimit loose objects or packLimit packs; temporary
// directories are looked for down to the configured sweep depth under the roots, where the
// deepest workspace is, and as deep as a repository's path goes under the pools.
type Keeper struct {
	pools      *pool.Pools
	runner     *git.Runner
	roots      []string
	settings   config.Housekeeping
	metrics    *metrics.Metrics
	looseLimit int
	packLimit  int
	poolCount  atomic.Int64
	poolBytes  atomic.Int64
}

// Totals are how many pools there were and the disk they took, as of the last round.
func (keeper *Keeper) Totals() (int, int64) {
	return int(keeper.poolCount.Load()), keeper.poolBytes.Load()
}

// New returns the keeper of pools, whose workspaces are made under roots.
func New(
	pools *pool.Pools, runner *git.Runner, roots []string, settings config.Housekeeping, measured *metrics.Metrics,
) *Keeper {
	return &Keeper{
		pools:      pools,
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
