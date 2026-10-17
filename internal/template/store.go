package template

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/lock"
	"github.com/dooservice/gitd/internal/pool"
)

// Builder checks a template out, as the workspace package builds a workspace.
type Builder interface {
	// BuildTemplate checks key out at building, a directory that does not exist yet and
	// that the store renames to final once it is complete.
	BuildTemplate(ctx context.Context, key Key, building, final string) error
	// ForgetTemplate undoes what building recorded elsewhere — the pools it borrows from —
	// just before the template at dir is removed.
	ForgetTemplate(ctx context.Context, key Key, dir string) error
}

// Store keeps the templates under Dir.
type Store struct {
	Dir     string
	locks   *lock.Locks
	builder Builder
}

// New returns the store of the templates under dir, built by builder.
func New(dir string, locks *lock.Locks, builder Builder) *Store {
	return &Store{Dir: dir, locks: locks, builder: builder}
}

// Take returns the template of key for the workspace at user, building it the first time,
// and notes that user uses it, so it is not retired under it.
func (store *Store) Take(ctx context.Context, key Key, user string) (string, error) {
	if err := key.Valid(); err != nil {
		return "", git.Fail(git.BadAddress, "%v", err)
	}
	dir := key.Dir(store.Dir)
	release, err := store.lock(ctx, dir)
	if err != nil {
		return "", err
	}
	defer release()
	if !exists(dir) {
		if err := store.build(ctx, key, dir); err != nil {
			return "", err
		}
	}
	return dir, addUser(dir, user)
}

// Let says the workspace at user no longer uses the template of key.
func (store *Store) Let(ctx context.Context, key Key, user string) error {
	dir := key.Dir(store.Dir)
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

// Users are the workspaces noted as using the template of key.
func (store *Store) Users(key Key) []string {
	return users(key.Dir(store.Dir))
}

// build checks the template out aside and moves it into place once complete, so that a
// template is never seen half built.
func (store *Store) build(ctx context.Context, key Key, dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	building := pool.Temp(dir)
	defer os.RemoveAll(building)
	if err := store.builder.BuildTemplate(ctx, key, building, dir); err != nil {
		return err
	}
	if err := os.Rename(building, dir); err != nil {
		return err
	}
	slog.Info("template built", "template", key.String())
	return nil
}

// Retire removes every template no workspace uses — none noted, or none of those noted
// still there — and nothing has taken for at least grace, and says how many it removed.
func (store *Store) Retire(ctx context.Context, grace time.Duration) int {
	retired := 0
	for _, key := range store.All() {
		if ctx.Err() != nil {
			break
		}
		if store.retire(ctx, key, grace) {
			retired++
		}
	}
	return retired
}

func (store *Store) retire(ctx context.Context, key Key, grace time.Duration) bool {
	dir := key.Dir(store.Dir)
	release, err := store.lock(ctx, dir)
	if err != nil {
		return false
	}
	defer release()
	inUse := slices.ContainsFunc(users(dir), func(path string) bool { return exists(path) })
	if inUse || time.Since(lastUsed(dir)) < grace {
		return false
	}
	if err := store.builder.ForgetTemplate(ctx, key, dir); err != nil {
		slog.Error("template not retired", "template", key.String(), "error", err)
		return false
	}
	if err := os.RemoveAll(dir); err != nil {
		slog.Error("template not retired", "template", key.String(), "error", err)
		return false
	}
	_ = os.Remove(usersOf(dir))
	slog.Info("template retired", "template", key.String())
	return true
}

// All are the keys of every template on disk, read back from where each lives.
func (store *Store) All() []Key {
	var keys []Key
	_ = filepath.WalkDir(store.Dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() || path == store.Dir {
			return nil
		}
		if pool.IsTemp(entry.Name()) {
			return filepath.SkipDir
		}
		key, ok := store.keyOf(path)
		if !ok {
			return nil
		}
		keys = append(keys, key)
		return filepath.SkipDir
	})
	return keys
}

// keyOf reads a template's key back from its directory, when the directory is one.
func (store *Store) keyOf(dir string) (Key, bool) {
	name := filepath.Base(dir)
	commit, submodules := strings.CutSuffix(name, "+submodules")
	if !fullCommit.MatchString(commit) {
		return Key{}, false
	}
	relative, err := filepath.Rel(store.Dir, filepath.Dir(dir))
	if err != nil {
		return Key{}, false
	}
	host, path, ok := strings.Cut(filepath.ToSlash(relative), "/")
	if !ok {
		return Key{}, false
	}
	key := Key{Commit: commit, Submodules: submodules}
	key.Repository.Host, key.Repository.Path = host, path
	return key, key.Dir(store.Dir) == dir
}

func (store *Store) lock(ctx context.Context, dir string) (func(), error) {
	release, err := store.locks.Acquire(ctx, "template:"+dir)
	if errors.Is(err, lock.ErrBusy) {
		return nil, git.Fail(git.Busy, "another operation holds this template")
	}
	if err != nil {
		return nil, fmt.Errorf("template lock: %w", err)
	}
	return release, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Sweep removes what a gitd that died left half built under the templates directory: the
// temporary directories templates are built in, and the users files left half written. It
// does not look inside a template, which has nothing of the sort and many files.
func (store *Store) Sweep() int {
	removed := 0
	_ = filepath.WalkDir(store.Dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == store.Dir {
			return nil
		}
		if pool.IsTemp(entry.Name()) {
			if os.RemoveAll(path) == nil {
				slog.Warn("half-built template removed", "path", path)
				removed++
			}
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if _, isTemplate := store.keyOf(path); isTemplate && entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	return removed
}
