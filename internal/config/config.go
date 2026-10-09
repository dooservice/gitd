// Package config reads gitd's configuration, a TOML file whose every key has a default.
package config

import (
	"errors"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is how one gitd runs: the Unix socket it answers on, the State directory that
// holds its pools, lock files and host keys, the Roots that are the only directories a
// workspace may be made in, the Git binary it runs, and where Metrics are served, if
// anywhere.
type Config struct {
	Socket  string   `toml:"socket"`
	State   string   `toml:"state"`
	Roots   []string `toml:"roots"`
	Git     string   `toml:"git"`
	Metrics string   `toml:"metrics"`

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
// borrows from is kept, and how old a git `.lock` must be to be taken as left behind.
type Housekeeping struct {
	Every       time.Duration `toml:"every"`
	RetireAfter time.Duration `toml:"retire_after"`
	StaleLock   time.Duration `toml:"stale_lock"`
}

// Default is the configuration of a customer's server.
func Default() Config {
	return Config{
		Socket: "/run/dooservice/gitd.sock",
		State:  "/var/lib/dooservice/gitd",
		Roots:  []string{"/var/lib/dooservice/projects"},
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
		},
	}
}

// Load reads the file at path over the defaults. A file that is not there is the defaults.
func Load(path string) (Config, error) {
	settings := Default()
	if _, err := toml.DecodeFile(path, &settings); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return settings, err
	}
	return settings, nil
}

// Pools is where the pools live.
func (settings Config) Pools() string {
	return filepath.Join(settings.State, "pools")
}

// Locks is where the lock files live.
func (settings Config) Locks() string {
	return filepath.Join(settings.State, "locks")
}
