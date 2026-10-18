//go:build linux

package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

func TestAWorkspaceCarryingItsPoolIsAcceptedOnlyWithThePoolTrulyMountedInside(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspaces(t, repository)
	ctx := context.Background()
	path := made(t, workspaces, roots, "pod", repository)
	objects, err := workspaces.carryPool(ctx, path, repository)
	if err != nil {
		t.Fatal(err)
	}
	if objects != filepath.Join(workspaces.pools.Path(repository), "objects") {
		t.Errorf("objects = %s", objects)
	}
	refused := func(why string) {
		t.Helper()
		_, err := workspaces.Describe(ctx, &gitdv1.DescribeRequest{Path: path})
		if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.ConfigRefused {
			t.Errorf("%s: %v", why, err)
		}
	}
	refused("nothing mounted")
	mounted := filepath.Join(path, ".git", poolMount)
	if err := os.Remove(mounted); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(objects, mounted); err != nil {
		t.Fatal(err)
	}
	refused("a link to the pool, not a mount")
	if err := os.Remove(mounted); err != nil {
		t.Fatal(err)
	}

	if os.Geteuid() != 0 {
		t.Skip("mounting the pool takes root")
	}
	if err := os.Mkdir(mounted, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount(objects, mounted, "", unix.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount("", mounted, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Unmount(mounted, unix.MNT_DETACH) })
	if _, err := workspaces.Describe(ctx, &gitdv1.DescribeRequest{Path: path}); err != nil {
		t.Errorf("with the pool mounted inside: %v", err)
	}
	// Mounted elsewhere, as in a pod, git still finds every object.
	elsewhere := t.TempDir()
	if err := unix.Mount(path, elsewhere, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Unmount(elsewhere, unix.MNT_DETACH) })
	if out := run(t, elsewhere, "log", "--oneline", "-1"); out == "" {
		t.Error("git found no commit in the workspace mounted elsewhere")
	}
	err = workspaces.Remove(ctx, &gitdv1.RemoveRequest{Path: path})
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.Busy {
		t.Errorf("removed a workspace with its pool still mounted inside: %v", err)
	}
}
