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
	"sync/atomic"
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
	// used is the disk the sandbox repositories took when last measured.
	used atomic.Int64
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
// show: its branches and tags are taken from the pool, its objects borrowed from it. A clone
// of it starts on branch, or on the repository's default branch when branch is empty.
func (sandboxes *Sandboxes) Make(ctx context.Context, repository repo.Repo, branch string) (Sandbox, error) {
	if branch != "" {
		if _, err := sandboxes.runner.Run(ctx, git.Cmd{Dir: "/", Args: []string{"check-ref-format", "refs/heads/" + branch}}); err != nil || strings.HasPrefix(branch, "-") {
			return Sandbox{}, git.Fail(git.BadAddress, "%q is not a branch name", branch)
		}
	}
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
	if branch == "" {
		branch = sandboxes.pools.DefaultBranch(ctx, repository)
	}
	if branch != "" {
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
		// The whole history: partial clones work, files arriving as they are read.
		return appendConfig(dir, "[uploadpack]\n\tallowFilter = true\n\tallowReachableSHA1InWant = true\n")
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

// RetireEvery removes, every so often until ctx ends, the sandbox repositories no token
// alive holds and nothing has pushed to for unusedFor.
func (sandboxes *Sandboxes) RetireEvery(ctx context.Context, every, unusedFor time.Duration, tokens *Tokens) {
	sandboxes.Measure()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sandboxes.Retire(ctx, unusedFor, tokens)
			sandboxes.Measure()
		}
	}
}

// Measure finds what disk the sandbox repositories take, which counts toward a tenant's
// disk limit.
func (sandboxes *Sandboxes) Measure() {
	var total int64
	_ = filepath.WalkDir(sandboxes.Dir, func(_ string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			if info, err := entry.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	sandboxes.used.Store(total)
}

// Used is the disk the sandbox repositories took when last measured.
func (sandboxes *Sandboxes) Used() int64 {
	return sandboxes.used.Load()
}

// Retire removes the sandbox repositories no token alive holds and nothing has pushed to
// for unusedFor, and says how many.
func (sandboxes *Sandboxes) Retire(ctx context.Context, unusedFor time.Duration, tokens *Tokens) int {
	held := map[string]bool{}
	for _, token := range tokens.Alive() {
		held[token.Sandbox] = true
	}
	retired := 0
	for _, id := range sandboxes.All() {
		if held[id] || time.Since(lastPush(sandboxes.Path(id))) < unusedFor {
			continue
		}
		if err := sandboxes.Remove(ctx, id); err == nil {
			retired++
		}
	}
	return retired
}

// lastPush is when anything last changed in a sandbox repository's refs, or when it was made.
func lastPush(dir string) time.Time {
	latest := time.Time{}
	newer := func(path string) {
		if info, err := os.Stat(path); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	newer(dir)
	newer(filepath.Join(dir, "packed-refs"))
	_ = filepath.WalkDir(filepath.Join(dir, "refs"), func(path string, _ fs.DirEntry, err error) error {
		if err == nil {
			newer(path)
		}
		return nil
	})
	return latest
}

// appendConfig adds lines to a repository's configuration, which only gitd writes.
func appendConfig(dir, lines string) error {
	file, err := os.OpenFile(filepath.Join(dir, "config"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(lines); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
