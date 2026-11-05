// Package mounthelper is gitd-mount: the one process that may mount, so that gitd needs no
// privilege at all. It runs as root, answers on a Unix socket only the user its
// configuration names, and does two things only — mount an overlay workspace and unmount
// one — inside directories that configuration fixes. The configuration is root's: gitd
// cannot widen what it may ask for.
//
// Every directory is opened with openat2(RESOLVE_NO_SYMLINKS) and the mount made through
// that descriptor, so a directory swapped for a symbolic link between the checks and the
// mount changes nothing: what was checked is what is mounted.
package mounthelper

import (
	"bufio"
	"fmt"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is who may ask, and where: templates are the only lower layers, layers the only
// place for upper and work directories, and roots the only places a workspace is mounted.
// Each may be given more than once: a gitd with tenants has templates, layers and roots for
// each.
type Config struct {
	UID       int
	Templates []string
	Layers    []string
	Roots     []string
}

func parse(lines *bufio.Scanner) (Config, error) {
	config := Config{UID: -1}
	for number := 1; lines.Scan(); number++ {
		line := strings.TrimSpace(lines.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, " ")
		value = strings.TrimSpace(value)
		if !ok || value == "" {
			return Config{}, fmt.Errorf("line %d: want a key and a value", number)
		}
		if key == "user" {
			uid, err := uidOf(value)
			if err != nil {
				return Config{}, fmt.Errorf("line %d: %w", number, err)
			}
			config.UID = uid
			continue
		}
		if !filepath.IsAbs(value) || filepath.Clean(value) != value || value == "/" {
			return Config{}, fmt.Errorf("line %d: %q is not a clean absolute directory", number, value)
		}
		switch key {
		case "templates":
			config.Templates = append(config.Templates, value)
		case "layers":
			config.Layers = append(config.Layers, value)
		case "root":
			config.Roots = append(config.Roots, value)
		default:
			return Config{}, fmt.Errorf("line %d: no key %q", number, key)
		}
	}
	if err := lines.Err(); err != nil {
		return Config{}, err
	}
	if config.UID <= 0 || len(config.Templates) == 0 || len(config.Layers) == 0 || len(config.Roots) == 0 {
		return Config{}, fmt.Errorf("want a user other than root, templates, layers and at least one root")
	}
	return config, nil
}

func uidOf(name string) (int, error) {
	if uid, err := strconv.Atoi(name); err == nil {
		return uid, nil
	}
	found, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(found.Uid)
}
