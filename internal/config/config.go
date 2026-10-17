// Package config reads gitd's configuration, a TOML file whose every key has a default.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is how one gitd runs: the Unix socket it answers on, the State directory that
// holds its pools, lock files and host keys, the Roots that are the only directories a
// workspace may be made in, the Git binary it runs, where Metrics are served, if
// anywhere, and the KnownHosts files whose SSH host keys are trusted besides the ones
// built in (GitHub, GitLab, Bitbucket and Codeberg): a self-hosted GitLab, Gitea or
// Forgejo goes there.
type Config struct {
	Socket     string             `toml:"socket"`
	State      string             `toml:"state"`
	Roots      []string           `toml:"roots"`
	Git        string             `toml:"git"`
	Metrics    string             `toml:"metrics"`
	KnownHosts []string           `toml:"known_hosts"`
	SSH        map[string]SSHHost `toml:"ssh"`

	Limits       Limits       `toml:"limits"`
	Housekeeping Housekeeping `toml:"housekeeping"`
}

// SSHHost is how a host takes SSH when not as `git` on port 22, as a Gitea or Forgejo
// behind `:2222`: `[ssh."git.example.com"] port = 2222`. Its host key then goes in a
// known_hosts file as `[git.example.com]:2222 ssh-ed25519 …`.
type SSHHost struct {
	Port int    `toml:"port"`
	User string `toml:"user"`
}

// Limits bound how much gitd does at once and for how long: fetches and writes running,
// operations queued behind them and for how long, and how long one call may take.
type Limits struct {
	Fetches   int           `toml:"fetches"`
	Writes    int           `toml:"writes"`
	Queued    int           `toml:"queued"`
	QueueWait time.Duration `toml:"queue_wait"`
	Operation time.Duration `toml:"operation"`
}

// Housekeeping is how often the pools are maintained, how long a pool no workspace
// borrows from is kept, how old a git `.lock` must be to be taken as left behind, and how
// many directories below a root the deepest workspace sits, which is how far down a
// start-up looks for what a crash left half built.
type Housekeeping struct {
	Every       time.Duration `toml:"every"`
	RetireAfter time.Duration `toml:"retire_after"`
	StaleLock   time.Duration `toml:"stale_lock"`
	SweepDepth  int           `toml:"sweep_depth"`
}

// DefaultSocket is where gitd answers when the configuration says nowhere else.
const DefaultSocket = "/run/gitd/gitd.sock"

// DefaultPath is the configuration file read when none is given.
const DefaultPath = "/etc/gitd/gitd.toml"

// Default is the configuration every key falls back to.
func Default() Config {
	return Config{
		Socket: DefaultSocket,
		State:  "/var/lib/gitd",
		Roots:  []string{"/var/lib/gitd/workspaces"},
		Git:    "git",
		Limits: Limits{
			Fetches:   4,
			Writes:    8,
			Queued:    32,
			QueueWait: 5 * time.Minute,
			Operation: 30 * time.Minute,
		},
		Housekeeping: Housekeeping{
			Every:       6 * time.Hour,
			RetireAfter: 30 * 24 * time.Hour,
			StaleLock:   time.Hour,
			SweepDepth:  4,
		},
	}
}

// Load reads the file at path over the defaults. A file that is not there is the defaults.
func Load(path string) (Config, error) {
	settings := Default()
	if _, err := toml.DecodeFile(path, &settings); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return settings, err
	}
	return settings, settings.check()
}

// check refuses an SSH host that could not be connected to, or whose user would read as
// an option or as part of the address.
func (settings Config) check() error {
	for host, ssh := range settings.SSH {
		if host == "" || strings.ContainsAny(host, ":/@ ") || strings.HasPrefix(host, "-") {
			return fmt.Errorf("ssh: %q is not a host name", host)
		}
		if ssh.Port < 0 || ssh.Port > 65535 {
			return fmt.Errorf("ssh.%q: port %d is not one", host, ssh.Port)
		}
		if strings.ContainsAny(ssh.User, ":/@ ") || strings.HasPrefix(ssh.User, "-") {
			return fmt.Errorf("ssh.%q: %q is not a user name", host, ssh.User)
		}
	}
	return nil
}

// Pools is where the pools live.
func (settings Config) Pools() string {
	return filepath.Join(settings.State, "pools")
}

// Locks is where the lock files live.
func (settings Config) Locks() string {
	return filepath.Join(settings.State, "locks")
}
