// Package userservice runs gitd for the person signed in, under their system's own manager
// of user services: systemd's user instance on Linux, launchd on macOS. gitd then starts
// with their session and is restarted when it fails, with nothing to keep a terminal open
// for.
package userservice

import "errors"

// Name is the service's name: gitd.service for systemd; launchd's label is Label.
const (
	Name  = "gitd"
	Label = "dev.gitd.gitd"
)

// ErrUnavailable is a system with no manager of user services to reach: a container, a
// session without systemd's user instance, an SSH session on macOS with no GUI login.
var ErrUnavailable = errors.New("no manager of user services here")

// Service is gitd run for one user: the Executable, the Config it reads, and the GitHub CLI
// whose token it is given as $GH_TOKEN each time it starts, when GitHubCLI names one.
type Service struct {
	Executable string
	Config     string
	GitHubCLI  string
}

// Manager starts gitd as a user service, and stops it.
type Manager interface {
	// Start installs the service to start with the user's session and starts it now,
	// restarting it when what it runs changed.
	Start(Service) error
	// Stop stops the service and removes it, so that it no longer starts.
	Stop() error
	// Installed says whether Start installed the service.
	Installed() bool
	// Log says where the service's output is read.
	Log() string
}

// tokenScript reads the GitHub CLI's token into $GH_TOKEN and becomes gitd, given gh, gitd
// and the configuration as its arguments $0, $1 and $2, so no path is ever part of the
// script. A gh logged out leaves $GH_TOKEN empty, and gitd says so as it refuses to start.
const tokenScript = `GH_TOKEN="$("$0" auth token)"; export GH_TOKEN; exec "$1" serve --config "$2"`

// command is the command line that starts gitd: itself, or sh reading the GitHub CLI's
// token first.
func (service Service) command() []string {
	if service.GitHubCLI == "" {
		return []string{service.Executable, "serve", "--config", service.Config}
	}
	return []string{"/bin/sh", "-c", tokenScript, service.GitHubCLI, service.Executable, service.Config}
}
