package template

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

// leftLayer is a layer a workspace left, holding one file.
func leftLayer(t *testing.T, content string) string {
	t.Helper()
	layer := filepath.Join(t.TempDir(), "upper")
	if err := os.MkdirAll(filepath.Join(layer, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layer, "node_modules", "left.js"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return layer
}

func TestAWarmLayerIsSealedTakenAndNotReplacedUnderAWorkspace(t *testing.T) {
	ctx := context.Background()
	store := newStore(t, &fakeBuilder{})
	// The layer moves within one filesystem, as layers and templates share gitd's state.
	store.Dir = filepath.Join(t.TempDir(), "templates")
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	sealed := Warm{Repository: repository, Name: "deps", Commit: commit, Changed: []string{"package-lock.json"}, Sealed: time.Now(), From: "/w/agent-0"}

	first := filepath.Join(filepath.Dir(store.Dir), "upper-1")
	if err := os.Rename(leftLayer(t, "one"), first); err != nil {
		t.Fatal(err)
	}
	if err := store.SealWarm(ctx, sealed, first); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Errorf("the layer was copied, not moved: %v", err)
	}
	user := filepath.Join(t.TempDir(), "agent-1")
	if err := os.MkdirAll(user, 0o755); err != nil {
		t.Fatal(err)
	}
	warm, err := store.TakeWarm(ctx, repository, "deps", user)
	if err != nil {
		t.Fatal(err)
	}
	if warm.Commit != commit || len(warm.Changed) != 1 || warm.From != "/w/agent-0" {
		t.Errorf("warm = %+v", warm)
	}
	if data, err := os.ReadFile(filepath.Join(warm.Dir, "node_modules", "left.js")); err != nil || string(data) != "one" {
		t.Errorf("the layer holds %q, %v", data, err)
	}

	second := filepath.Join(filepath.Dir(store.Dir), "upper-2")
	if err := os.Rename(leftLayer(t, "two"), second); err != nil {
		t.Fatal(err)
	}
	if err := store.SealWarm(ctx, sealed, second); !hasReason(err, git.Busy) {
		t.Errorf("replaced a layer a workspace stands on: %v", err)
	}
	if err := store.LetWarm(ctx, warm.Dir, user); err != nil {
		t.Fatal(err)
	}
	if err := store.SealWarm(ctx, sealed, second); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(warm.Dir, "node_modules", "left.js")); string(data) != "two" {
		t.Errorf("sealing again did not replace the layer: %q", data)
	}

	if keys := store.All(); len(keys) != 0 {
		t.Errorf("a warm layer was taken for a template: %v", keys)
	}
	if warms := store.Warms(repository); len(warms) != 1 || warms[0].Name != "deps" {
		t.Errorf("warms = %+v", warms)
	}
	if removed := store.Sweep() + store.SweepWarm(); removed != 0 {
		t.Errorf("sweeping removed %d things from a sealed layer", removed)
	}
	if retired := store.RetireWarm(ctx, time.Hour); retired != 0 {
		t.Errorf("retired a layer used just now: %d", retired)
	}
	if retired := store.RetireWarm(ctx, 0); retired != 1 {
		t.Errorf("retired %d unused layers", retired)
	}
	if _, err := store.TakeWarm(ctx, repository, "deps", user); !hasReason(err, git.NotFound) {
		t.Errorf("a retired layer was taken: %v", err)
	}
}

func TestAWarmLayerNameIsAWordAPathCanCarry(t *testing.T) {
	for _, name := range []string{"", "Deps", "../x", "a/b", ".hidden", "-flag", string(make([]byte, 65))} {
		if ValidWarmName(name) == nil {
			t.Errorf("%q passed as a warm layer name", name)
		}
	}
	for _, name := range []string{"deps", "node-22", "py3.13_lock"} {
		if err := ValidWarmName(name); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
}

func hasReason(err error, reason git.Reason) bool {
	failure, ok := git.AsFailure(err)
	return ok && failure.Reason == reason
}
