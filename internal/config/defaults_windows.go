//go:build windows

package config

// DefaultMountHelper names no helper: Windows has no gitd-mount.
const DefaultMountHelper = ""

// DefaultSocket is where gitd answers when the configuration says nowhere else.
const DefaultSocket = `C:\ProgramData\gitd\gitd.sock`

// DefaultPath is the configuration file read when none is given.
const DefaultPath = `C:\ProgramData\gitd\gitd.toml`

// DefaultState is where pools, templates and workspaces live unless configured. Workspaces
// are cloned by blocks only on a Dev Drive or another ReFS volume, so state and roots are
// best moved to one.
const DefaultState = `C:\ProgramData\gitd`
