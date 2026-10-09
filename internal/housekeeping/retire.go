package housekeeping

import (
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dooservice/gitd/internal/pool"
)

// retire removes a pool that no workspace has borrowed from for long enough. A workspace
// counts while its alternates still point at the pool, wherever the registry says it is:
// an environment deleted by hand no longer does.
func (keeper *Keeper) retire(dir string) bool {
	objects := filepath.Join(dir, "objects")
	borrowed := slices.ContainsFunc(pool.Workspaces(dir), func(path string) bool {
		return borrows(path, objects)
	})
	if borrowed || time.Since(pool.LastUsed(dir)) < keeper.settings.RetireAfter {
		return false
	}
	if err := os.RemoveAll(dir); err != nil {
		slog.Error("pool not retired", "pool", dir, "error", err)
		return false
	}
	slog.Info("pool retired", "pool", dir)
	return true
}

// borrows is whether the repository at path, or one of its submodules, reads objects from
// a pool's objects directory.
func borrows(path, objects string) bool {
	alternates, _ := filepath.Glob(filepath.Join(path, ".git", "modules", "*", "objects", "info", "alternates"))
	alternates = append(alternates, filepath.Join(path, ".git", "objects", "info", "alternates"))
	return slices.ContainsFunc(alternates, func(file string) bool {
		data, err := os.ReadFile(file)
		return err == nil && slices.Contains(strings.Fields(string(data)), objects)
	})
}
