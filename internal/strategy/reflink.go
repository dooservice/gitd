package strategy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// ReflinkMaker makes a workspace by cloning every file of the template: the clones share
// the template's blocks until written, so the copy takes almost no disk. It takes about as
// long as writing the files would — it saves disk, not time — so it is used only when
// configured. Modification times are kept, which is what lets git trust the template's
// index with core.checkStat=minimal.
type ReflinkMaker struct{}

// Name is Reflink.
func (ReflinkMaker) Name() Name { return Reflink }

// Make clones the template into dest, built aside first by the caller's convention: on any
// failure dest is removed.
func (ReflinkMaker) Make(ctx context.Context, template, dest string) error {
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	}
	err := filepath.WalkDir(template, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		relative, err := filepath.Rel(template, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			return clone(path, target, info)
		}
		return nil
	})
	if err != nil {
		_ = os.RemoveAll(dest)
	}
	return err
}

// Unmake removes the clone.
func (ReflinkMaker) Unmake(_ context.Context, dest string) error {
	return os.RemoveAll(dest)
}

// clone makes target share source's blocks, with source's mode and modification time.
func clone(source, target string, info fs.FileInfo) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if err := unix.IoctlFileClone(int(out.Fd()), int(in.Fd())); err != nil {
		_ = out.Close()
		if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EXDEV) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOTTY) {
			return fmt.Errorf("%w: %v", ErrUnsupported, err)
		}
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(target, info.ModTime(), info.ModTime())
}
