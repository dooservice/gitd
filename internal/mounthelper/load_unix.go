//go:build !windows

package mounthelper

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Load reads the configuration at path, which must be root's and writable by root only:
// whoever could write it could mount anywhere.
//
//	user gitd
//	templates /var/lib/gitd/templates
//	layers /var/lib/gitd/layers
//	root /var/lib/gitd/workspaces
//
// So must every directory above it, or whoever could write one could put another file in
// its place. The file is opened once, without following a link, and checked as opened.
func Load(path string) (Config, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Config{}, err
	}
	if !rootsOnly(info) {
		return Config{}, fmt.Errorf("%s must be root's and writable by root only", path)
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		above, err := os.Lstat(dir)
		if err != nil || !above.IsDir() || !rootsOnly(above) && !rootsSticky(above) {
			return Config{}, fmt.Errorf("%s, above %s, must be a directory root's and writable by root only", dir, path)
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	return parse(bufio.NewScanner(file))
}

// rootsOnly says whether a file or directory is root's and writable by no one else.
func rootsOnly(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0 && info.Mode().Perm()&0o022 == 0
}

// rootsSticky says whether a directory is root's with the sticky bit, as /tmp is: others may
// write in it, but cannot remove or replace a file of root's there.
func rootsSticky(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0 && info.Mode()&os.ModeSticky != 0
}
