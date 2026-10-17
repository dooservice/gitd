package workspace

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// The modes git records a file with.
const (
	regularMode    = "100644"
	executableMode = "100755"
	symlinkMode    = "120000"
	gitlinkMode    = "160000"
)

// changed is one file changed against the base commit: its path, how, and its object
// there, when it was there.
type changed struct {
	path       string
	kind       gitdv1.ChangedFile_Kind
	baseObject string
}

// compare reads each candidate as the base commit has it and as the workspace shows it,
// through root, which never follows a link out of the workspace, and keeps those that
// differ. A directory stands for every file under it; what is inside a submodule is left
// to the submodule.
func (workspaces *Workspaces) compare(
	ctx context.Context, repository repo.Repo, base string, candidates []string, root *os.Root,
) ([]changed, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	entries, err := workspaces.pools.TreeEntries(ctx, repository, base, candidates)
	if err != nil {
		return nil, err
	}
	submodules, err := workspaces.pools.Submodules(ctx, repository, base)
	if err != nil {
		return nil, err
	}
	format, err := workspaces.pools.ObjectFormat(ctx, repository)
	if err != nil {
		return nil, err
	}
	paths, err := expandDirectories(root, candidates)
	if err != nil {
		return nil, err
	}
	for path := range entries {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	var files []changed
	for _, path := range slices.Compact(paths) {
		entry, inBase := entries[path]
		if insideAny(path, submodules) || entry.Mode == gitlinkMode || !inBase && hasUnder(entries, path) {
			continue
		}
		object, mode, inWorkspace, err := workspaceObject(root, path, format)
		if err != nil {
			return nil, err
		}
		switch {
		case inBase && !inWorkspace:
			files = append(files, changed{path: path, kind: gitdv1.ChangedFile_KIND_DELETED, baseObject: entry.Object})
		case !inBase && inWorkspace:
			files = append(files, changed{path: path, kind: gitdv1.ChangedFile_KIND_ADDED})
		case inBase && inWorkspace && (object != entry.Object || mode != entry.Mode):
			files = append(files, changed{path: path, kind: gitdv1.ChangedFile_KIND_MODIFIED, baseObject: entry.Object})
		}
	}
	return files, nil
}

// expandDirectories replaces each candidate the workspace shows as a directory — as git
// lists an ignored one whole — with the files under it, walked inside the workspace
// without following a link.
func expandDirectories(root *os.Root, candidates []string) ([]string, error) {
	var paths []string
	for _, candidate := range candidates {
		candidate = strings.TrimSuffix(candidate, "/")
		info, err := root.Lstat(filepath.FromSlash(candidate))
		if err != nil || !info.IsDir() {
			paths = append(paths, candidate)
			continue
		}
		err = fs.WalkDir(root.FS(), candidate, func(walked string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				paths = append(paths, walked)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// workspaceObject is a file as the workspace shows it — its object id and mode as git would
// record them — and false when the workspace shows no file there.
func workspaceObject(root *os.Root, path, format string) (string, string, bool, error) {
	native := filepath.FromSlash(path)
	info, err := root.Lstat(native)
	if errors.Is(err, fs.ErrNotExist) || err == nil && info.IsDir() {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	digest := newDigest(format)
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := root.Readlink(native)
		if err != nil {
			return "", "", false, err
		}
		fmt.Fprintf(digest, "blob %d\x00%s", len(target), target)
		return hex.EncodeToString(digest.Sum(nil)), symlinkMode, true, nil
	case info.Mode().IsRegular():
		file, err := root.Open(native)
		if err != nil {
			return "", "", false, err
		}
		defer file.Close()
		fmt.Fprintf(digest, "blob %d\x00", info.Size())
		if _, err := io.Copy(digest, file); err != nil {
			return "", "", false, err
		}
		mode := regularMode
		if info.Mode()&0o111 != 0 {
			mode = executableMode
		}
		return hex.EncodeToString(digest.Sum(nil)), mode, true, nil
	default:
		return "", "", false, nil
	}
}

func newDigest(format string) hash.Hash {
	if format == "sha256" {
		return sha256.New()
	}
	return sha1.New()
}

// insideAny says whether path is one of prefixes or under one.
func insideAny(path string, prefixes []string) bool {
	return slices.ContainsFunc(prefixes, func(prefix string) bool {
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	})
}

// hasUnder says whether the base has files under path, which makes path a directory.
func hasUnder(entries map[string]pool.TreeEntry, path string) bool {
	for entry := range entries {
		if strings.HasPrefix(entry, path+"/") {
			return true
		}
	}
	return false
}

// withoutIgnored leaves out the files added that the template's .gitignore — the base
// commit's — ignores, as git, run in the template, says.
func (workspaces *Workspaces) withoutIgnored(ctx context.Context, lower string, files []changed) ([]changed, error) {
	var added []string
	for _, file := range files {
		if file.kind == gitdv1.ChangedFile_KIND_ADDED {
			added = append(added, file.path)
		}
	}
	ignored := map[string]bool{}
	if len(added) == 0 {
		return files, nil
	}
	out, err := workspaces.runner.Run(ctx, git.Cmd{Dir: lower, Differences: true,
		Args: []string{"check-ignore", "-z", "--stdin"}, Input: strings.Join(added, "\x00") + "\x00"})
	if err != nil {
		return nil, err
	}
	for name := range strings.SplitSeq(out, "\x00") {
		if name != "" {
			ignored[name] = true
		}
	}
	return slices.DeleteFunc(files, func(file changed) bool { return ignored[file.path] }), nil
}
