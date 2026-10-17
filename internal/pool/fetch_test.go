package pool

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
)

func TestARemoteThatCannotBeReadSaysWhatGitSaid(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "web.git")
	if out, err := exec.Command("git", "init", "--bare", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	pools := &Pools{runner: git.NewRunner("git")}
	// Nothing listens on port 1: the remote cannot be read at all.
	access := credentials.Access{URL: "https://127.0.0.1:1/acme/web"}

	err := pools.fetch(context.Background(), dir, access, Want{}, true, nil)

	failure, ok := git.AsFailure(err)
	if !ok || failure.GitSaid == "the repository names no default branch" {
		t.Fatalf("fetch = %v; want what git said about the remote", err)
	}
}
