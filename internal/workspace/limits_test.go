package workspace

import (
	"context"
	"path/filepath"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/repo"
)

func TestATenantMakesNoWorkspaceBeyondItsLimits(t *testing.T) {
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	workspaces, roots := newTestWorkspaces(t, repository)
	made(t, workspaces, roots, "agent-1", repository)
	ctx := context.Background()
	asked := &gitdv1.PrepareRequest{Url: repository.HTTPS(), Path: filepath.Join(roots, "agent-2")}

	workspaces.UseLimits(Limits{Workspaces: 1})
	_, err := workspaces.Prepare(ctx, asked, nil)
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.LimitReached {
		t.Errorf("a second workspace past a limit of one: %v", err)
	}
	workspaces.UseLimits(Limits{Disk: 100, Used: func() int64 { return 100 }})
	_, err = workspaces.Prepare(ctx, asked, nil)
	if failure, ok := git.AsFailure(err); !ok || failure.Reason != git.LimitReached {
		t.Errorf("a workspace with the disk full: %v", err)
	}
	workspaces.UseLimits(Limits{Workspaces: 5, Disk: 100, Used: func() int64 { return 10 }})
	if err := workspaces.withinLimits(); err != nil {
		t.Errorf("within its limits, refused: %v", err)
	}
}
