package endpoint

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// sandboxName is what a sandbox repository is called: its ID, and nothing a path could
// make more of.
var sandboxName = regexp.MustCompile(`^sbx_[0-9a-f]{16}$`)

// Sandboxes are the sandbox repositories under Dir: one bare repository per sandbox,
// borrowing its pool's objects, holding the pool's branches as they were when it was made
// and whatever the sandbox pushed. gitd writes their configuration; a sandbox reaches them
// only through git's smart HTTP, which cannot.
type Sandboxes struct {
	Dir    string
	runner *git.Runner
	pools  *pool.Pools
}

// NewSandboxes returns the sandbox repositories under dir.
func NewSandboxes(dir string, runner *git.Runner, pools *pool.Pools) *Sandboxes {
	return &Sandboxes{Dir: dir, runner: runner, pools: pools}
}

// Sandbox is one sandbox repository: its ID, the repository it is of, and when it was made.
type Sandbox struct {
	ID         string
	Repository repo.Repo
	Made       time.Time
}

// Make makes a sandbox repository of repository, whose pool must hold what it should
// show: its branches and tags are taken from the pool, its objects borrowed from it.
func (sandboxes *Sandboxes) Make(ctx context.Context, repository repo.Repo) (Sandbox, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return Sandbox{}, err
	}
	id := "sbx_" + hex.EncodeToString(random)
	dir := sandboxes.Path(id)
	if err := os.MkdirAll(sandboxes.Dir, 0o755); err != nil {
		return Sandbox{}, err
	}
	building := pool.Temp(dir)
	defer os.RemoveAll(building)
	if _, err := sandboxes.runner.Run(ctx, git.Cmd{Dir: sandboxes.Dir, Args: []string{"init", "--bare", "--quiet", building}}); err != nil {
		return Sandbox{}, err
	}
	if err := sandboxes.borrow(building, repository); err != nil {
		return Sandbox{}, err
	}
	if err := sandboxes.runner.SetConfig(ctx, building,
		"gitd.url", repository.HTTPS(),
		"gc.auto", "0",
		"receive.denyDeletes", "true",
		"receive.denyNonFastForwards", "true",
		"receive.fsckObjects", "true",
	); err != nil {
		return Sandbox{}, err
	}
	poolDir := sandboxes.pools.Path(repository)
	if _, err := sandboxes.runner.Run(ctx, git.Cmd{Dir: building, Local: true, Args: []string{
		"fetch", "--no-prune", "--update-shallow", "--end-of-options", poolDir,
		"+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*",
	}}); err != nil {
		return Sandbox{}, git.About(err, repository.HTTPS())
	}
	if branch := sandboxes.pools.DefaultBranch(ctx, repository); branch != "" {
		if _, err := sandboxes.runner.Run(ctx, git.Cmd{Dir: building, Args: []string{"symbolic-ref", "HEAD", "refs/heads/" + branch}}); err != nil {
			return Sandbox{}, err
		}
	}
	if err := os.Rename(building, dir); err != nil {
		return Sandbox{}, err
	}
	if err := sandboxes.pools.Use(ctx, repository, dir); err != nil {
		_ = os.RemoveAll(dir)
		return Sandbox{}, err
	}
	return Sandbox{ID: id, Repository: repository, Made: time.Now()}, nil
}

// borrow points a sandbox repository at its pool's objects, and where its history stops.
func (sandboxes *Sandboxes) borrow(dir string, repository repo.Repo) error {
	objects := filepath.Join(sandboxes.pools.Path(repository), "objects") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "objects", "info", "alternates"), []byte(objects), 0o644); err != nil {
		return err
	}
	from := sandboxes.pools.Shallow(repository)
	if from == "" {
		return nil
	}
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "shallow"), data, 0o644)
}

// Path is where the sandbox repository with this ID is, whether it exists or not.
func (sandboxes *Sandboxes) Path(id string) string {
	return filepath.Join(sandboxes.Dir, id+".git")
}

// Get reads back the sandbox repository with this ID.
func (sandboxes *Sandboxes) Get(ctx context.Context, id string) (Sandbox, error) {
	if !sandboxName.MatchString(id) {
		return Sandbox{}, git.Fail(git.BadAddress, "%q is not a sandbox", id)
	}
	dir := sandboxes.Path(id)
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return Sandbox{}, git.Fail(git.NotFound, "there is no sandbox %s", id)
	}
	if err != nil {
		return Sandbox{}, err
	}
	url, err := sandboxes.runner.Config(ctx, filepath.Join(dir, "config"), "gitd.url")
	if err != nil {
		return Sandbox{}, err
	}
	repository, ok := repo.Of(strings.TrimSpace(url))
	if !ok {
		return Sandbox{}, fmt.Errorf("sandbox %s names no repository", id)
	}
	return Sandbox{ID: id, Repository: repository, Made: info.ModTime()}, nil
}

// Remove deletes a sandbox repository, and takes it off its pool.
func (sandboxes *Sandboxes) Remove(ctx context.Context, id string) error {
	sandbox, err := sandboxes.Get(ctx, id)
	if err != nil {
		return err
	}
	dir := sandboxes.Path(id)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return sandboxes.pools.Forget(ctx, sandbox.Repository, dir)
}

// All are the IDs of every sandbox repository.
func (sandboxes *Sandboxes) All() []string {
	entries, err := os.ReadDir(sandboxes.Dir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, entry := range entries {
		if id, ok := strings.CutSuffix(entry.Name(), ".git"); ok && sandboxName.MatchString(id) {
			ids = append(ids, id)
		}
	}
	return ids
}
