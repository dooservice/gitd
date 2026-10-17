// Package config reads gitd's configuration, a TOML file whose every key has a default.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/dooservice/gitd/internal/repo"
)

// Config is how one gitd runs: the Unix socket it answers on, the State directory that
// holds its pools, lock files and host keys, the Roots that are the only directories a
// workspace may be made in, the Git binary it runs, where Metrics are served, if
// anywhere, and the KnownHosts files whose SSH host keys are trusted besides the ones
// built in (GitHub, GitLab, Bitbucket and Codeberg): a self-hosted GitLab, Gitea or
// Forgejo goes there; and the Hosts reached on other ports, or over SSH as another user,
// than the usual ones: `[hosts."git.example.com"] https_port = 3000, ssh_port = 2222`. A
// host whose SSH is on another port has its key in a known_hosts file as
// `[git.example.com]:2222 ssh-ed25519 …`.
type Config struct {
	Socket     string     `toml:"socket"`
	State      string     `toml:"state"`
	Roots      []string   `toml:"roots"`
	Git        string     `toml:"git"`
	Metrics    string     `toml:"metrics"`
	KnownHosts []string   `toml:"known_hosts"`
	Hosts      repo.Hosts `toml:"hosts"`

	Limits       Limits       `toml:"limits"`
	Housekeeping Housekeeping `toml:"housekeeping"`
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
	return settings, settings.normalize()
}

// name is what a host or an SSH user may be: nothing that reads as an option, or as part
// of an address or of a shell word.
var name = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// normalize lowercases the hosts, as repositories name them, and refuses one that is not a
// host name, is written twice, or has a port or user it could not be reached with.
func (settings *Config) normalize() error {
	hosts := make(repo.Hosts, len(settings.Hosts))
	for written, host := range settings.Hosts {
		lowered := strings.ToLower(written)
		if !name.MatchString(lowered) {
			return fmt.Errorf("hosts: %q is not a host name", written)
		}
		if _, twice := hosts[lowered]; twice {
			return fmt.Errorf("hosts: %q is written twice", lowered)
		}
		for _, port := range []int{host.HTTPSPort, host.SSHPort} {
			if port < 0 || port > 65535 {
				return fmt.Errorf("hosts.%q: port %d is not one", written, port)
			}
		}
		if host.SSHUser != "" && !name.MatchString(host.SSHUser) {
			return fmt.Errorf("hosts.%q: %q is not a user name", written, host.SSHUser)
		}
		hosts[lowered] = host
	}
	settings.Hosts = hosts
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
