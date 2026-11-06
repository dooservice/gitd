//go:build darwin

package userservice

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ForSystem is launchd's domain of the user's login session, when there is one: an SSH
// session with nobody logged in at the screen has none.
func ForSystem() (Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	manager := launchd{
		domain: "gui/" + strconv.Itoa(os.Getuid()),
		plist:  filepath.Join(home, "Library", "LaunchAgents", Label+".plist"),
		log:    filepath.Join(home, "Library", "Logs", "gitd.log"),
	}
	if err := manager.launchctl("print", manager.domain); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return manager, nil
}

// launchd manages gitd as the agent whose property list is plist, in the login session's
// domain, logging to log.
type launchd struct {
	domain string
	plist  string
	log    string
}

func (manager launchd) Start(service Service) error {
	text := []byte(launchAgent{service: service, log: manager.log}.String())
	written, err := os.ReadFile(manager.plist)
	changed := err != nil || !bytes.Equal(written, text)
	loaded := manager.loaded()
	if !changed && loaded {
		return manager.launchctl("kickstart", manager.target())
	}
	if loaded {
		if err := manager.launchctl("bootout", manager.target()); err != nil {
			return err
		}
	}
	for _, dir := range []string{filepath.Dir(manager.plist), filepath.Dir(manager.log)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(manager.plist, text, 0o644); err != nil {
		return err
	}
	return manager.launchctl("bootstrap", manager.domain, manager.plist)
}

func (manager launchd) Stop() error {
	if manager.loaded() {
		if err := manager.launchctl("bootout", manager.target()); err != nil {
			return err
		}
	}
	if err := os.Remove(manager.plist); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (manager launchd) Installed() bool {
	_, err := os.Stat(manager.plist)
	return err == nil
}

func (manager launchd) Log() string { return manager.log }

// target is the agent within the domain.
func (manager launchd) target() string { return manager.domain + "/" + Label }

// loaded says whether launchd has the agent.
func (manager launchd) loaded() bool { return manager.launchctl("print", manager.target()) == nil }

// launchctl runs launchctl, failing with what it said.
func (launchd) launchctl(args ...string) error {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	if err != nil {
		said := strings.TrimSpace(string(out))
		if said == "" {
			said = err.Error()
		}
		return fmt.Errorf("launchctl %s: %s", strings.Join(args, " "), said)
	}
	return nil
}
