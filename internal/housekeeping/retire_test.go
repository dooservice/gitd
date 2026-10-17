package housekeeping

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dooservice/gitd/internal/config"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/metrics"
	"github.com/dooservice/gitd/internal/pool"
)

func aPool(t *testing.T, pools string, name string, lastUsed time.Time, borrowedBy string) string {
	t.Helper()
	dir := filepath.Join(pools, "github.com", "acme", name+".git")
	if err := os.MkdirAll(filepath.Join(dir, "objects", "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "objects", "pack", "p.pack"), make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(dir, "gitd-workspaces")
	if err := os.WriteFile(registry, []byte(borrowedBy+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(registry, lastUsed, lastUsed); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRetiringOnRequestTakesOnlyWhatNobodyUsedForThatLong(t *testing.T) {
	state := t.TempDir()
	pools := filepath.Join(state, "pools")
	workspace := filepath.Join(state, "projects", "acme", "prod", "app")
	borrowedPool := aPool(t, pools, "borrowed", time.Now().Add(-60*24*time.Hour), workspace)
	if err := os.MkdirAll(filepath.Join(workspace, ".git", "objects", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	alternates := filepath.Join(borrowedPool, "objects") + "\n"
	if err := os.WriteFile(filepath.Join(workspace, ".git", "objects", "info", "alternates"), []byte(alternates), 0o644); err != nil {
		t.Fatal(err)
	}
	old := aPool(t, pools, "old", time.Now().Add(-20*24*time.Hour), "")
	recent := aPool(t, pools, "recent", time.Now().Add(-2*24*time.Hour), "")

	locks, err := lock.NewLocks(filepath.Join(state, "locks"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	keeper := &Keeper{
		pools:    pool.New(pools, nil, nil, locks, lock.NewQueue(1, 1, time.Minute), time.Minute),
		settings: config.Housekeeping{RetireAfter: 30 * 24 * time.Hour},
		metrics:  metrics.New(func() float64 { return 0 }),
	}
	retired, freed := keeper.Retire(context.Background(), 15*24*time.Hour)

	if retired != 1 || freed < 1000 {
		t.Fatalf("retired %d pools, freed %d bytes", retired, freed)
	}
	for dir, kept := range map[string]bool{borrowedPool: true, old: false, recent: true} {
		if _, err := os.Stat(dir); (err == nil) != kept {
			t.Errorf("%s kept = %v, want %v", filepath.Base(dir), err == nil, kept)
		}
	}
}
