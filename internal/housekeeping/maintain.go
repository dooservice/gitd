package housekeeping

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/repo"
)

// maintain packs the refs and writes the commit graph of a pool every time, and repacks
// it only when it has piled up enough to be worth it. What comes back is the disk the pool
// takes afterwards.
func (keeper *Keeper) maintain(ctx context.Context, dir string, repository repo.Repo) int64 {
	lock.ClearStale(dir, keeper.settings.StaleLock)
	steps := [][]string{{"pack-refs", "--all"}, {"commit-graph", "write", "--reachable"}}
	if loose, packs := keeper.counts(ctx, dir); loose >= keeper.looseLimit || packs >= keeper.packLimit {
		repack := []string{"repack", "--cruft", "--cruft-expiration=never", "-d", "--write-bitmap-index"}
		steps = append([][]string{repack}, steps...)
	}
	for _, step := range steps {
		if _, err := keeper.runner.Run(ctx, git.Cmd{Dir: dir, Args: step}); err != nil {
			slog.Error("pool maintenance failed", "repository", repository.HTTPS(), "step", step[0], "error", err)
			break
		}
	}
	taken := size(filepath.Join(dir, "objects"))
	keeper.metrics.PoolBytes.WithLabelValues(repository.HTTPS()).Set(float64(taken))
	return taken
}

// counts reads `count-objects -v`: the loose objects and the packs.
func (keeper *Keeper) counts(ctx context.Context, dir string) (loose, packs int) {
	out, err := keeper.runner.Run(ctx, git.Cmd{Dir: dir, Args: []string{"count-objects", "-v"}})
	if err != nil {
		return 0, 0
	}
	for line := range strings.Lines(out) {
		key, value, _ := strings.Cut(strings.TrimSpace(line), ": ")
		number, _ := strconv.Atoi(value)
		switch key {
		case "count":
			loose = number
		case "packs":
			packs = number
		}
	}
	return loose, packs
}

// size is what the files under dir take.
func size(dir string) int64 {
	var total int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}
