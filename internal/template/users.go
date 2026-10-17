package template

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/dooservice/gitd/internal/pool"
)

// The users of a template are kept in a file beside its directory, never inside it: inside,
// it would show in every workspace made from the template. The file's time is when the
// template was last used.

func usersOf(dir string) string { return dir + ".users" }

// users are the paths of the workspaces noted as using the template at dir.
func users(dir string) []string {
	file, err := os.Open(usersOf(dir))
	if err != nil {
		return nil
	}
	defer file.Close()
	var paths []string
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		if line := strings.TrimSpace(lines.Text()); line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

func writeUsers(dir string, paths []string) error {
	text := ""
	if len(paths) > 0 {
		text = strings.Join(paths, "\n") + "\n"
	}
	return pool.WriteAtomic(usersOf(dir), []byte(text))
}

func addUser(dir, path string) error {
	paths := users(dir)
	if !slices.Contains(paths, path) {
		paths = append(paths, path)
	}
	return writeUsers(dir, paths)
}

func removeUser(dir, path string) error {
	return writeUsers(dir, slices.DeleteFunc(users(dir), func(noted string) bool { return noted == path }))
}

// lastUsed is when the template at dir was last taken or let go of.
func lastUsed(dir string) time.Time {
	info, err := os.Stat(usersOf(dir))
	if errors.Is(err, fs.ErrNotExist) || err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
