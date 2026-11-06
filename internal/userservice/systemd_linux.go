//go:build linux

package userservice

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ForSystem is systemd's user instance, when the session has one to reach.
func ForSystem() (Manager, error) {
	manager := systemd{}
	if err := manager.systemctl("show-environment"); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	manager.unit = filepath.Join(dir, "systemd", "user", Name+".service")
	return manager, nil
}

// systemd manages gitd as the user unit at unit.
type systemd struct {
	unit string
}

func (manager systemd) Start(service Service) error {
	text := []byte(systemdUnit{service: service}.String())
	written, err := os.ReadFile(manager.unit)
	changed := err != nil || !bytes.Equal(written, text)
	if changed {
		if err := os.MkdirAll(filepath.Dir(manager.unit), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(manager.unit, text, 0o644); err != nil {
			return err
		}
		if err := manager.systemctl("daemon-reload"); err != nil {
			return err
		}
	}
	if err := manager.systemctl("enable", Name+".service"); err != nil {
		return err
	}
	if changed {
		return manager.systemctl("restart", Name+".service")
	}
	return manager.systemctl("start", Name+".service")
}

func (manager systemd) Stop() error {
	if err := manager.systemctl("disable", "--now", Name+".service"); err != nil {
		return err
	}
	if err := os.Remove(manager.unit); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return manager.systemctl("daemon-reload")
}

func (manager systemd) Installed() bool {
	_, err := os.Stat(manager.unit)
	return err == nil
}

func (systemd) Log() string { return "journalctl --user -u " + Name }

// systemctl runs systemctl on the user's instance, failing with what it said.
func (systemd) systemctl(args ...string) error {
	out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
	if err != nil {
		said := strings.TrimSpace(string(out))
		if said == "" {
			said = err.Error()
		}
		return fmt.Errorf("systemctl --user %s: %s", strings.Join(args, " "), said)
	}
	return nil
}
