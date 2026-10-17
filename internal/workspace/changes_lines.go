package workspace

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/git"
)

// largestCopied is the largest file whose lines are counted; a larger one counts as binary.
const largestCopied = 16 << 20

// lineCount is how many lines a changed file adds and removes, or that it is binary.
type lineCount struct {
	added, removed int32
	binary         bool
}

// aside is a private directory where the changed files are copied — the base commit's
// under a/, the workspace's under b/ — for git to compare with --no-index: git never reads
// in the workspace itself, and a link there is copied as what it points at, never followed.
type aside struct {
	directory string
	large     map[string]bool
}

// copyAside copies the given files of a change set aside.
func (workspaces *Workspaces) copyAside(ctx context.Context, set changeSet, files []changed) (*aside, error) {
	directory, err := os.MkdirTemp("", "gitd-changes-*")
	if err != nil {
		return nil, err
	}
	copied := &aside{directory: directory, large: map[string]bool{}}
	root, err := os.OpenRoot(set.path)
	if err != nil {
		copied.remove()
		return nil, err
	}
	defer root.Close()
	for _, file := range files {
		if file.baseObject != "" {
			content, err := workspaces.pools.Blob(ctx, set.repository, file.baseObject)
			if err != nil {
				copied.remove()
				return nil, err
			}
			if err := copied.write("a", file.path, strings.NewReader(content)); err != nil {
				copied.remove()
				return nil, err
			}
		}
		if file.kind != gitdv1.ChangedFile_KIND_DELETED {
			if err := copied.fromWorkspace(root, file.path); err != nil {
				copied.remove()
				return nil, err
			}
		}
	}
	return copied, nil
}

// fromWorkspace copies a workspace's file under b/: a link as the path it names.
func (copied *aside) fromWorkspace(root *os.Root, path string) error {
	native := filepath.FromSlash(path)
	info, err := root.Lstat(native)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := root.Readlink(native)
		if err != nil {
			return err
		}
		return copied.write("b", path, strings.NewReader(target))
	}
	if info.Size() > largestCopied {
		copied.large[path] = true
		return copied.write("b", path, strings.NewReader("\x00"))
	}
	file, err := root.Open(native)
	if err != nil {
		return err
	}
	defer file.Close()
	return copied.write("b", path, file)
}

func (copied *aside) write(side, path string, content io.Reader) error {
	target := filepath.Join(copied.directory, side, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// side is a file's path in a/ or b/, or /dev/null when that side has none.
func (copied *aside) side(side, path string) string {
	if _, err := os.Lstat(filepath.Join(copied.directory, side, filepath.FromSlash(path))); err != nil {
		return "/dev/null"
	}
	return side + "/" + path
}

func (copied *aside) remove() {
	_ = os.RemoveAll(copied.directory)
}

// countLines counts each changed file's lines with git, in one call over the files copied
// aside.
func (workspaces *Workspaces) countLines(ctx context.Context, set changeSet) (map[string]lineCount, error) {
	counted := map[string]lineCount{}
	if len(set.files) == 0 {
		return counted, nil
	}
	copied, err := workspaces.copyAside(ctx, set, set.files)
	if err != nil {
		return nil, err
	}
	defer copied.remove()
	out, err := workspaces.runner.Run(ctx, git.Cmd{Dir: copied.directory, Differences: true, Args: []string{
		"diff", "--no-index", "--no-renames", "--no-ext-diff", "--no-textconv", "--numstat", "-z", "--", "a", "b",
	}})
	if err != nil {
		return nil, err
	}
	fields := strings.Split(out, "\x00")
	for index := 0; index+2 < len(fields); index += 3 {
		numbers, source, target := fields[index], fields[index+1], fields[index+2]
		path := strings.TrimPrefix(target, "b/")
		if target == "/dev/null" {
			path = strings.TrimPrefix(source, "a/")
		}
		added, removed, _ := strings.Cut(strings.TrimSuffix(numbers, "\t"), "\t")
		if added == "-" || copied.large[path] {
			counted[path] = lineCount{binary: true}
			continue
		}
		addedLines, _ := strconv.Atoi(added)
		removedLines, _ := strconv.Atoi(removed)
		counted[path] = lineCount{added: int32(addedLines), removed: int32(removedLines)}
	}
	return counted, nil
}
