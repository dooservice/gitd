//go:build !windows

package config

// DefaultMountHelper is where gitd-mount answers unless the configuration says otherwise.
const DefaultMountHelper = "/run/gitd-mount/gitd-mount.sock"

// DefaultSocket is where gitd answers when the configuration says nowhere else.
const DefaultSocket = "/run/gitd/gitd.sock"

// DefaultPath is the configuration file read when none is given.
const DefaultPath = "/etc/gitd/gitd.toml"

// DefaultState is where pools, templates and workspaces live unless configured.
const DefaultState = "/var/lib/gitd"
