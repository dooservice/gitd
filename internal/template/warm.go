package template

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// WarmDir is the directory under the templates where warm layers live. The templates'
// own walks never enter it.
const WarmDir = ".warm"

// warmName is what a warm layer may be called: a word a path and a flag can carry.
var warmName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// Warm is a layer sealed from a workspace — what installing its dependencies wrote — that
// later workspaces of the same repository and commit stand on, between their template and
// their own layer. It is named per repository, and never changes once sealed: sealing the
// name again replaces it, once no workspace stands on it.
type Warm struct {
	Repository repo.Repo
	Name       string
	// Dir is the layer itself, mounted read-only.
	Dir string
	// Commit is the commit of the template it was sealed on: it stands on that one only.
	Commit string
	// Changed are the paths the layer changes that git does not ignore, as compared with
	// its template, so that what a workspace changed is read without walking the layer.
	Changed []string
	// Sealed is when it was sealed, and From the workspace it was sealed from.
	Sealed time.Time
	From   string
}

// warmManifest is what is kept beside a warm layer about it.
type warmManifest struct {
	Commit  string    `json:"commit"`
	Changed []string  `json:"changed"`
	Sealed  time.Time `json:"sealed"`
	From    string    `json:"from"`
}

// ValidWarmName says why name cannot name a warm layer, or nothing when it can.
func ValidWarmName(name string) error {
	if !warmName.MatchString(name) {
		return git.Fail(git.BadAddress, "%q is not a warm layer name: lowercase letters, digits, dots, dashes and underscores, 64 at most", name)
	}
	return nil
}

// warmDir is where the warm layer name of repository lives:
// `.warm/<host>/<path>/<name>`, with `<name>.json` and `<name>.users` beside it.
func (store *Store) warmDir(repository repo.Repo, name string) string {
	return filepath.Join(store.Dir, WarmDir, repository.Host, filepath.FromSlash(repository.Path), name)
}

func manifestOf(dir string) string { return dir + ".json" }

// SealWarm makes from, a layer a workspace left, the warm layer name of repository, by
// moving it, not copying: what overlay marked in it stays as it marked it. A warm layer
// of that name a workspace still stands on is not replaced.
func (store *Store) SealWarm(ctx context.Context, sealed Warm, from string) error {
	if err := ValidWarmName(sealed.Name); err != nil {
		return err
	}
	dir := store.warmDir(sealed.Repository, sealed.Name)
	release, err := store.lock(ctx, dir)
	if err != nil {
		return err
	}
	defer release()
	if store.inUse(dir) {
		return git.Fail(git.Busy, "the warm layer %s is in use: a workspace stands on it", sealed.Name)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	manifest, err := json.Marshal(warmManifest{Commit: sealed.Commit, Changed: sealed.Changed, Sealed: sealed.Sealed, From: sealed.From})
	if err != nil {
		return err
	}
	old := pool.Temp(dir)
	if err := os.Rename(dir, old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	defer os.RemoveAll(old)
	if err := os.Rename(from, dir); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			return git.Fail(git.Unsupported, "a warm layer is moved, never copied: the layers and the templates must be on one filesystem")
		}
		return err
	}
	if err := pool.WriteAtomic(manifestOf(dir), manifest); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	slog.Info("warm layer sealed", "repository", sealed.Repository.HTTPS(), "name", sealed.Name, "commit", sealed.Commit)
	return writeUsers(dir, nil)
}

// TakeWarm returns the warm layer name of repository for the workspace at user, and notes
// that user stands on it, so it is neither replaced nor retired under it.
func (store *Store) TakeWarm(ctx context.Context, repository repo.Repo, name, user string) (Warm, error) {
	if err := ValidWarmName(name); err != nil {
		return Warm{}, err
	}
	dir := store.warmDir(repository, name)
	release, err := store.lock(ctx, dir)
	if err != nil {
		return Warm{}, err
	}
	defer release()
	warm, err := store.readWarm(repository, name)
	if err != nil {
		return Warm{}, err
	}
	return warm, addUser(dir, user)
}

// LetWarm says the workspace at user no longer stands on the warm layer at dir.
func (store *Store) LetWarm(ctx context.Context, dir, user string) error {
	release, err := store.lock(ctx, dir)
	if err != nil {
		return err
	}
	defer release()
	if !exists(dir) {
		return nil
	}
	return removeUser(dir, user)
}

// Warms are the warm layers of repository.
func (store *Store) Warms(repository repo.Repo) []Warm {
	entries, err := os.ReadDir(store.warmDir(repository, ""))
	if err != nil {
		return nil
	}
	var warms []Warm
	for _, entry := range entries {
		if !entry.IsDir() || ValidWarmName(entry.Name()) != nil {
			continue
		}
		if warm, err := store.readWarm(repository, entry.Name()); err == nil {
			warms = append(warms, warm)
		}
	}
	return warms
}

// readWarm reads back the warm layer name of repository and what is kept about it.
func (store *Store) readWarm(repository repo.Repo, name string) (Warm, error) {
	dir := store.warmDir(repository, name)
	data, err := os.ReadFile(manifestOf(dir))
	if errors.Is(err, fs.ErrNotExist) || !exists(dir) {
		return Warm{}, git.Fail(git.NotFound, "%s has no warm layer %s", repository.HTTPS(), name)
	}
	if err != nil {
		return Warm{}, err
	}
	var manifest warmManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Warm{}, fmt.Errorf("warm layer %s: %w", name, err)
	}
	return Warm{
		Repository: repository, Name: name, Dir: dir,
		Commit: manifest.Commit, Changed: manifest.Changed, Sealed: manifest.Sealed, From: manifest.From,
	}, nil
}

// inUse says whether a workspace noted as standing on the layer at dir is still there.
func (store *Store) inUse(dir string) bool {
	return slices.ContainsFunc(users(dir), func(path string) bool { return exists(path) })
}

// RetireWarm removes every warm layer no workspace stands on and nothing has taken for at
// least grace, and says how many it removed.
func (store *Store) RetireWarm(ctx context.Context, grace time.Duration) int {
	retired := 0
	for _, manifest := range store.manifests() {
		if ctx.Err() != nil {
			break
		}
		if store.retireWarm(ctx, strings.TrimSuffix(manifest, ".json"), grace) {
			retired++
		}
	}
	return retired
}

func (store *Store) retireWarm(ctx context.Context, dir string, grace time.Duration) bool {
	release, err := store.lock(ctx, dir)
	if err != nil {
		return false
	}
	defer release()
	if store.inUse(dir) || time.Since(lastUsed(dir)) < grace {
		return false
	}
	if err := os.RemoveAll(dir); err != nil {
		slog.Error("warm layer not retired", "path", dir, "error", err)
		return false
	}
	_ = os.Remove(manifestOf(dir))
	_ = os.Remove(usersOf(dir))
	slog.Info("warm layer retired", "path", dir)
	return true
}

// manifests are the files kept beside every warm layer, found without entering a layer.
func (store *Store) manifests() []string {
	var found []string
	root := filepath.Join(store.Dir, WarmDir)
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}
		if entry.IsDir() && exists(manifestOf(path)) {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			found = append(found, path)
		}
		return nil
	})
	return found
}

// SweepWarm removes what a gitd that died left half done among the warm layers: a layer
// set aside to be replaced, or a file half written. It never enters a layer.
func (store *Store) SweepWarm() int {
	removed := 0
	root := filepath.Join(store.Dir, WarmDir)
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}
		if pool.IsTemp(entry.Name()) {
			if os.RemoveAll(path) == nil {
				slog.Warn("half-done warm layer removed", "path", path)
				removed++
			}
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() && exists(manifestOf(path)) {
			return filepath.SkipDir
		}
		return nil
	})
	return removed
}
