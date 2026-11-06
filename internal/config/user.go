package config

import (
	"os"
	"path/filepath"
)

// User is where a gitd run for one person on their own machine keeps things, by their
// system's convention: its configuration file, its State, the Socket it answers on, and the
// Root workspaces are made in, ~/gitd/workspaces on every system.
type User struct {
	Config string
	State  string
	Socket string
	Root   string
}

// ForUser is the current user's places; false where gitd does not run per user, as on
// Windows, whose gitd is the service, or when there is no home directory.
func ForUser() (User, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return User{}, false
	}
	return userPlaces(environment{home: home, variable: os.Getenv})
}

// environment is what the places are read from: the home directory and the environment's
// variables.
type environment struct {
	home     string
	variable func(string) string
}

// xdg is the places on Linux and the other Unix systems, by the XDG Base Directory
// specification: ~/.config/gitd/gitd.toml, ~/.local/state/gitd, and the socket in the
// session's runtime directory, /run/user/UID, or under the state when there is none.
func (env environment) xdg() User {
	state := filepath.Join(env.directory("XDG_STATE_HOME", ".local", "state"), "gitd")
	socket := filepath.Join(state, "gitd.sock")
	if runtime := env.variable("XDG_RUNTIME_DIR"); filepath.IsAbs(runtime) {
		socket = filepath.Join(runtime, "gitd", "gitd.sock")
	}
	return User{
		Config: filepath.Join(env.directory("XDG_CONFIG_HOME", ".config"), "gitd", "gitd.toml"),
		State:  state,
		Socket: socket,
		Root:   env.root(),
	}
}

// library is the places on macOS, all in ~/Library/Application Support/gitd: the
// configuration, the state beside it, and the socket, a path short enough for one.
func (env environment) library() User {
	support := filepath.Join(env.home, "Library", "Application Support", "gitd")
	return User{
		Config: filepath.Join(support, "gitd.toml"),
		State:  filepath.Join(support, "state"),
		Socket: filepath.Join(support, "gitd.sock"),
		Root:   env.root(),
	}
}

// directory is the directory the variable names, when it is an absolute path as the XDG
// specification requires, else the one under the home directory.
func (env environment) directory(variable string, underHome ...string) string {
	if dir := env.variable(variable); filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(append([]string{env.home}, underHome...)...)
}

// root is where workspaces are made: ~/gitd/workspaces, in sight and on the home's
// filesystem, as the state is, so that templates can be cloned into it.
func (env environment) root() string {
	return filepath.Join(env.home, "gitd", "workspaces")
}
