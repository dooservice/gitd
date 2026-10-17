package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/credentials"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
	"github.com/dooservice/gitd/internal/strategy"
	"github.com/dooservice/gitd/internal/template"
)

// treeMounter stands in for an overlay without privilege: "mounting" copies the lower
// layers' trees into the target, the topmost last so it wins, keeping modification times,
// and unmounting empties it.
type treeMounter struct{}

func (treeMounter) MountOverlay(lowers []string, _, _, target string) error {
	for index := len(lowers) - 1; index >= 0; index-- {
		if err := exec.Command("cp", "-a", lowers[index]+"/.", target).Run(); err != nil {
			return err
		}
	}
	return nil
}

func (treeMounter) Unmount(target string) error {
	entries, _ := os.ReadDir(target)
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(target, entry.Name()))
	}
	return nil
}

func TestAWorkspaceMadeFromATemplateIsOnItsBranchAndUnmadeOnRemove(t *testing.T) {
	root := t.TempDir()
	upstream, pools, roots := filepath.Join(root, "upstream"), filepath.Join(root, "pools"), filepath.Join(root, "workspaces")
	repository := repo.Repo{Host: "github.com", Path: "acme/web"}
	run(t, root, "init", "-q", "-b", "main", upstream)
	if err := os.WriteFile(filepath.Join(upstream, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, upstream, "add", "README.md")
	run(t, upstream, "commit", "-q", "-m", "first")
	tip := run(t, upstream, "rev-parse", "HEAD")
	run(t, root, "clone", "-q", "--bare", upstream, repository.Dir(pools))
	if err := os.MkdirAll(roots, 0o755); err != nil {
		t.Fatal(err)
	}

	runner := git.NewRunner("git")
	locks, err := lock.NewLocks(filepath.Join(root, "locks"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := credentials.NewKeys(filepath.Join(root, "state"), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	workspaces := New([]string{roots}, runner, pool.New(pools, runner, keys, locks, nil, 0), locks, lock.NewQueue(4, 4, time.Minute), time.Hour)
	templates := template.New(filepath.Join(root, "templates"), locks, workspaces)
	maker := &strategy.OverlayMaker{Layers: filepath.Join(root, "layers"), Mounter: treeMounter{}}
	workspaces.UseTemplates(templates, maker)
	ctx := context.Background()

	agent := func(name string) job {
		return job{path: filepath.Join(roots, name), repository: repository, revision: onBranch("main")}
	}
	if !workspaces.fromTemplate(agent("agent-1")) {
		t.Fatal("a plain workspace is not made from a template")
	}
	result, err := workspaces.createFromTemplate(ctx, agent("agent-1"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Commit != tip || result.Branch != "main" {
		t.Errorf("result = %+v; want %s on main", result, tip)
	}
	if result.Strategy != gitdv1.Strategy_STRATEGY_OVERLAY || result.Layers == nil || result.Layers.GetLower() == "" {
		t.Errorf("the result does not say it was made by overlay, with its layers: %+v", result)
	}
	described, err := workspaces.Describe(ctx, &gitdv1.DescribeRequest{Path: agent("agent-1").path})
	if err != nil {
		t.Fatal(err)
	}
	if described.GetBranch() != "main" || described.GetCommit() != tip || described.GetModified() || described.GetUrl() != repository.HTTPS() {
		t.Errorf("described = %+v", described)
	}
	if described.GetStrategy() != gitdv1.Strategy_STRATEGY_OVERLAY || described.GetLayers().GetUpper() == "" {
		t.Errorf("describe does not say it was made by overlay: %+v", described)
	}
	if head := run(t, agent("agent-1").path, "symbolic-ref", "HEAD"); head != "refs/heads/main" {
		t.Errorf("HEAD is %s, not refs/heads/main", head)
	}
	if remote := run(t, agent("agent-1").path, "rev-parse", "refs/remotes/origin/main"); remote != tip {
		t.Errorf("origin/main is %s, not %s", remote, tip)
	}
	if status := run(t, agent("agent-1").path, "status", "--porcelain"); status != "" {
		t.Errorf("a fresh workspace is dirty: %q", status)
	}

	if _, err := workspaces.createFromTemplate(ctx, agent("agent-2")); err != nil {
		t.Fatal(err)
	}
	key := template.Key{Repository: repository, Commit: tip}
	if users := templates.Users(key); len(users) != 2 {
		t.Errorf("template users = %v; want both workspaces", users)
	}

	if err := workspaces.Remove(ctx, &gitdv1.RemoveRequest{Path: agent("agent-1").path}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(agent("agent-1").path); err == nil {
		t.Error("the removed workspace is still there")
	}
	if users := templates.Users(key); len(users) != 1 || users[0] != agent("agent-2").path {
		t.Errorf("template users after remove = %v", users)
	}
	if len(maker.Records()) != 1 {
		t.Errorf("layers left = %v; want agent-2's only", maker.Records())
	}

	full := agent("agent-3")
	full.full = true
	if workspaces.fromTemplate(full) {
		t.Error("a whole-history workspace would be made from a template, which holds one commit")
	}
	for _, asked := range []strategy.Name{strategy.Copy, strategy.Reflink} {
		other := agent("agent-4")
		other.strategy = asked
		if workspaces.fromTemplate(other) {
			t.Errorf("asked for %s, the workspace would be made by overlay", asked)
		}
	}

	written, err := workspaces.create(ctx, agent("agent-5"))
	if err != nil {
		t.Fatal(err)
	}
	if written.Strategy != gitdv1.Strategy_STRATEGY_COPY || written.Layers != nil {
		t.Errorf("a written workspace's result = %+v", written)
	}
	if made, _ := workspaces.made(ctx, agent("agent-5").path); made != gitdv1.Strategy_STRATEGY_COPY {
		t.Errorf("a written workspace reads as %v", made)
	}
}
