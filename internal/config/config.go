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
	Socket      string       `toml:"socket"`
	State       string       `toml:"state"`
	Roots       []string     `toml:"roots"`
	Git         string       `toml:"git"`
	Metrics     string       `toml:"metrics"`
	KnownHosts  []string     `toml:"known_hosts"`
	Hosts       repo.Hosts   `toml:"hosts"`
	Credentials []Credential `toml:"credentials"`
	// SecretsKeyFile names the key the secrets of connections made through the API are
	// encrypted with, under the state directory: 32 bytes, raw, hex or base64, best a
	// systemd credential. Without one they are kept in files only gitd's user reads.
	SecretsKeyFile string `toml:"secrets_key_file"`

	GitHub       GitHub       `toml:"github"`
	Workspaces   Workspaces   `toml:"workspaces"`
	Publish      Publish      `toml:"publish"`
	Audit        Audit        `toml:"audit"`
	Limits       Limits       `toml:"limits"`
	Housekeeping Housekeeping `toml:"housekeeping"`
	Endpoint     Endpoint     `toml:"endpoint"`
	// Tenants are the customers of a platform hosting many on one gitd, each isolated from
	// the others; without any, gitd is one implicit tenant, as before 0.9.
	Tenants []Tenant `toml:"tenants"`
	// APIKeyFile is the key a caller on the network brings when there are no tenants; its
	// network address answers the API only with one.
	APIKeyFile string `toml:"api_key_file"`
}

// Credential is one credential gitd keeps for the repositories under Prefix (empty for
// any), so that a caller need not send it: a token read from TokenFile or from the
// environment variable TokenEnv, sent as TokenUser, the SSH key at SSHKey, or a GitHub App
// whose installation tokens open one repository each. The configuration names where the
// secret is, never the secret: a TokenFile or an App's key is best a systemd credential
// (`LoadCredential=`), which lives in memory.
type Credential struct {
	Prefix    string     `toml:"prefix"`
	TokenFile string     `toml:"token_file"`
	TokenEnv  string     `toml:"token_env"`
	TokenUser string     `toml:"token_user"`
	SSHKey    string     `toml:"ssh_key"`
	GitHubApp *GitHubApp `toml:"github_app"`
}

// GitHubApp is a GitHub App by its ID and the file of its private key, read each time it
// signs, so replacing the file rotates the key.
type GitHubApp struct {
	AppID          int64  `toml:"app_id"`
	PrivateKeyFile string `toml:"private_key_file"`
}

// GitHub is gitd's public GitHub App, which people sign in through: its client ID, which
// is not a secret, and its name in addresses. A GitHub Enterprise Server needs an App of
// its own, registered there.
type GitHub struct {
	LoginClientID string `toml:"login_client_id"`
	LoginApp      string `toml:"login_app"`
}

// loginClientID and loginApp are the public App people sign in through, by its client ID,
// which is not a secret, and its name in addresses: set when gitd is built, from the
// GITD_LOGIN_CLIENT_ID and GITD_LOGIN_APP variables (`make build`, the release, the image).
// A gitd built without them offers no sign-in until [github] names an App.
var (
	loginClientID string
	loginApp      string
)

// Workspaces is how new workspaces are made. Strategy "" (or "auto") lets gitd pick: an
// overlay on a shared template when one can be mounted, else every file written; "overlay"
// and "copy" ask for one, and "reflink" clones the template to save disk, not time.
type Workspaces struct {
	Strategy string `toml:"strategy"`
	// MountHelper is the socket of gitd-mount, which mounts overlays for a gitd that may
	// not; the default is used when it exists.
	MountHelper string `toml:"mount_helper"`
}

// Publish is the rules of publishing. BranchPrefix, when set, is what every published
// branch must start with, e.g. "agents/": with publishing already limited to new branches
// and never forced, it keeps agents off every branch people use. Settings are the rules of
// the repositories under a prefix, and Presets the named sets of rules settings take, as
// the API's PublishingService sets them; those of the file are shown by the API and never
// changed through it.
type Publish struct {
	BranchPrefix string           `toml:"branch_prefix"`
	Presets      []PublishPreset  `toml:"presets"`
	Settings     []PublishSetting `toml:"settings"`
}

// PublishPreset is a named set of rules: paths a publish may not change, paths it may not
// delete, and at most how many files it may change. One named "recommended" replaces gitd's.
type PublishPreset struct {
	Name            string   `toml:"name"`
	ProtectedPaths  []string `toml:"protected_paths"`
	KeptPaths       []string `toml:"kept_paths"`
	MaxChangedFiles *int32   `toml:"max_changed_files"`
}

// PublishSetting is the rules of publishing for the repositories under Prefix (empty for
// every one): the Preset it takes, by name, paths a publish may not change and paths it may
// not delete, as git's glob pathspecs, paths its own rules leave free, at most how many
// files it may change, the branch prefix, and how pull requests are opened.
type PublishSetting struct {
	Prefix          string       `toml:"prefix"`
	Preset          string       `toml:"preset"`
	ProtectedPaths  []string     `toml:"protected_paths"`
	KeptPaths       []string     `toml:"kept_paths"`
	AllowedPaths    []string     `toml:"allowed_paths"`
	MaxChangedFiles *int32       `toml:"max_changed_files"`
	BranchPrefix    string       `toml:"branch_prefix"`
	PullRequests    PullRequests `toml:"pull_requests"`
}

// PullRequests is how publishes open pull requests: Open is "when_asked" or "always", and
// RequestReviewFromRequester asks the person a publish names to review it.
type PullRequests struct {
	Open                       string   `toml:"open"`
	Draft                      bool     `toml:"draft"`
	Labels                     []string `toml:"labels"`
	RequestReviewFromRequester bool     `toml:"request_review_from_requester"`
}

// Audit is how the audit log, every publish done or refused, is kept under the state
// directory: its file is rotated when it reaches RotateBytes, and a rotated file is deleted
// once older than Keep.
type Audit struct {
	RotateBytes int64         `toml:"rotate_bytes"`
	Keep        time.Duration `toml:"keep"`
}

// DefaultMountHelper is where gitd-mount answers unless the configuration says otherwise.
const DefaultMountHelper = "/run/gitd-mount/gitd-mount.sock"

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
	// TemplateGrace is how long a template no workspace uses is kept, in case one is made
	// from it again soon.
	TemplateGrace time.Duration `toml:"template_grace"`
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
		GitHub: GitHub{LoginClientID: loginClientID, LoginApp: loginApp},
		Audit: Audit{
			RotateBytes: 16 << 20,
			Keep:        365 * 24 * time.Hour,
		},
		Limits: Limits{
			Fetches:   4,
			Writes:    8,
			Queued:    32,
			QueueWait: 5 * time.Minute,
			Operation: 30 * time.Minute,
		},
		Housekeeping: Housekeeping{
			Every:         6 * time.Hour,
			RetireAfter:   30 * 24 * time.Hour,
			StaleLock:     time.Hour,
			SweepDepth:    4,
			TemplateGrace: time.Hour,
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

// branchPrefix is what a branch prefix may be: the start of a branch name git accepts, in
// letters, digits and . _ - /, never starting with one that reads as an option or a path.
var branchPrefix = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._/-]*$`)

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
	switch settings.Workspaces.Strategy {
	case "", "auto", "overlay", "copy", "reflink":
	default:
		return fmt.Errorf("workspaces: strategy %q is none of auto, overlay, copy and reflink", settings.Workspaces.Strategy)
	}
	if prefix := settings.Publish.BranchPrefix; !ValidBranchPrefix(prefix) {
		return fmt.Errorf("publish: branch_prefix %q cannot start a branch name", prefix)
	}
	if err := settings.Endpoint.normalize(); err != nil {
		return err
	}
	if settings.Audit.RotateBytes <= 0 || settings.Audit.Keep <= 0 {
		return fmt.Errorf("audit: rotate_bytes and keep must be more than zero")
	}
	if err := settings.normalizeCredentials(); err != nil {
		return err
	}
	return settings.normalizeTenants()
}

// normalizeCredentials checks that each credential names exactly one secret.
func (settings *Config) normalizeCredentials() error {
	for index, credential := range settings.Credentials {
		set := 0
		for _, source := range []string{credential.TokenFile, credential.TokenEnv, credential.SSHKey} {
			if source != "" {
				set++
			}
		}
		if credential.GitHubApp != nil {
			set++
		}
		if set != 1 {
			return fmt.Errorf("credentials[%d] (%q): give exactly one of token_file, token_env, ssh_key and github_app",
				index, credential.Prefix)
		}
		if credential.TokenUser != "" && (credential.SSHKey != "" || credential.GitHubApp != nil) {
			return fmt.Errorf("credentials[%d] (%q): token_user goes with a token", index, credential.Prefix)
		}
		if app := credential.GitHubApp; app != nil && (app.AppID <= 0 || app.PrivateKeyFile == "") {
			return fmt.Errorf("credentials[%d] (%q): a github_app needs its app_id and private_key_file", index, credential.Prefix)
		}
	}
	return nil
}

// ValidBranchPrefix says whether prefix can start a branch name git accepts; empty is no
// prefix, and valid.
func ValidBranchPrefix(prefix string) bool {
	return prefix == "" ||
		branchPrefix.MatchString(prefix) && !strings.Contains(prefix, "..") && !strings.Contains(prefix, "//")
}

// Pools is where the pools live.
func (settings Config) Pools() string {
	return filepath.Join(settings.State, "pools")
}

// Templates is where the templates copy-on-write workspaces are made from live.
func (settings Config) Templates() string {
	return filepath.Join(settings.State, "templates")
}

// AuditLog is where the audit log is kept.
func (settings Config) AuditLog() string {
	return filepath.Join(settings.State, "audit")
}

// Connections is where the connections made through the API are kept.
func (settings Config) Connections() string {
	return filepath.Join(settings.State, "connections")
}

// PublishingSettings is where the publishing settings set through the API are kept.
func (settings Config) PublishingSettings() string {
	return filepath.Join(settings.State, "publishing")
}

// Locks is where the lock files live.
func (settings Config) Locks() string {
	return filepath.Join(settings.State, "locks")
}
