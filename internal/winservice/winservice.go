//go:build windows

// Package winservice runs gitd as a Windows service, as systemd runs it on Linux: started
// with the machine, restarted when it fails, answering whoever is in the local group gitd.
//
// The service runs as LocalSystem. What it makes is reached through the group, as a socket
// readable by a group is on Linux: members may connect to the socket, read the pools and
// templates their workspaces borrow from, and change only the workspaces themselves.
package winservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Name is the service's name, and Group the local group whose members may use it.
const (
	Name  = "gitd"
	Group = "gitd"
)

// Running says whether this process was started by the service manager.
func Running() bool {
	running, err := svc.IsWindowsService()
	return err == nil && running
}

// Run answers the service manager until serve returns, which it must when its context ends:
// the context ends when the service is stopped or the machine shuts down.
func Run(serve func(context.Context) error) error {
	handler := &handler{serve: serve}
	if err := svc.Run(Name, handler); err != nil {
		return err
	}
	return handler.err
}

type handler struct {
	serve func(context.Context) error
	err   error
}

func (handler *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- handler.serve(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			handler.err = err
			if err != nil {
				return true, 1
			}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				status <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
			}
		}
	}
}

// Install registers gitd at executable as a service started with the machine and restarted
// when it fails, serving with the configuration at configPath. It creates the group and adds
// member to it, and gives the group its access to state and roots.
func Install(executable, configPath, member, state string, roots []string) error {
	if err := ensureGroup(member); err != nil {
		return err
	}
	if err := grant(state, roots); err != nil {
		return err
	}
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to the service manager, which needs an administrator: %w", err)
	}
	defer manager.Disconnect()
	if service, err := manager.OpenService(Name); err == nil {
		service.Close()
		return fmt.Errorf("the service %s is already installed: uninstall it first", Name)
	}
	service, err := manager.CreateService(Name, executable, mgr.Config{
		DisplayName: "gitd",
		Description: "Instant, secretless git checkouts for agent sandboxes",
		StartType:   mgr.StartAutomatic,
	}, "serve", "--config", configPath)
	if err != nil {
		return err
	}
	defer service.Close()
	restart := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: time.Minute},
	}
	return service.SetRecoveryActions(restart, uint32((24 * time.Hour).Seconds()))
}

// Uninstall stops the service if it runs and removes it. The group and what gitd made are
// left: removing the service is not removing the workspaces.
func Uninstall() error {
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to the service manager, which needs an administrator: %w", err)
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(Name)
	if err != nil {
		return fmt.Errorf("the service %s is not installed", Name)
	}
	defer service.Close()
	if _, err := service.Control(svc.Stop); err == nil {
		waitFor(service, svc.Stopped)
	}
	return service.Delete()
}

// Start starts the installed service and waits until it runs.
func Start() error {
	return withService(func(service *mgr.Service) error {
		if err := service.Start(); err != nil {
			return err
		}
		if !waitFor(service, svc.Running) {
			return errors.New("the service did not start: see gitd.log beside its configuration")
		}
		return nil
	})
}

// Stop stops the service and waits until it has.
func Stop() error {
	return withService(func(service *mgr.Service) error {
		if _, err := service.Control(svc.Stop); err != nil {
			return err
		}
		waitFor(service, svc.Stopped)
		return nil
	})
}

// State says whether the service is installed and whether it runs.
func State() (string, error) {
	state := "not installed"
	err := withService(func(service *mgr.Service) error {
		status, err := service.Query()
		if err != nil {
			return err
		}
		state = describe(status.State)
		return nil
	})
	if err != nil && strings.Contains(err.Error(), "not installed") {
		return state, nil
	}
	return state, err
}

// GroupSID is the group's security identifier, when the group exists.
func GroupSID() (*windows.SID, bool) {
	sid, _, kind, err := windows.LookupSID("", Group)
	if err != nil || kind != windows.SidTypeAlias && kind != windows.SidTypeGroup {
		return nil, false
	}
	return sid, true
}

func withService(do func(*mgr.Service) error) error {
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to the service manager, which needs an administrator: %w", err)
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(Name)
	if err != nil {
		return fmt.Errorf("the service %s is not installed", Name)
	}
	defer service.Close()
	return do(service)
}

func waitFor(service *mgr.Service, want svc.State) bool {
	for range 60 {
		status, err := service.Query()
		if err == nil && status.State == want {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func describe(state svc.State) string {
	switch state {
	case svc.Running:
		return "running"
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "starting"
	case svc.StopPending:
		return "stopping"
	}
	return fmt.Sprintf("state %d", state)
}

// ensureGroup creates the local group if it is missing and adds member, which may already be
// in it. `net localgroup` is what Windows itself offers for both.
func ensureGroup(member string) error {
	if _, ok := GroupSID(); !ok {
		if out, err := exec.Command("net", "localgroup", Group, "/add", "/comment:May use gitd").CombinedOutput(); err != nil {
			return fmt.Errorf("creating the group %s: %v: %s", Group, err, strings.TrimSpace(string(out)))
		}
	}
	if member == "" {
		return nil
	}
	out, err := exec.Command("net", "localgroup", Group, member, "/add").CombinedOutput()
	if err != nil && !strings.Contains(string(out), "1378") {
		return fmt.Errorf("adding %s to the group %s: %v: %s", member, Group, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// grant makes state SYSTEM's and the administrators', readable by the group, and each root
// changeable by it: what the service makes there inherits it.
func grant(state string, roots []string) error {
	group, ok := GroupSID()
	if !ok {
		return fmt.Errorf("the group %s does not exist", Group)
	}
	if err := protect(state, fmt.Sprintf("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1200a9;;;%s)", group)); err != nil {
		return err
	}
	for _, root := range roots {
		if err := protect(root, fmt.Sprintf("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1301bf;;;%s)", group)); err != nil {
			return err
		}
	}
	return nil
}

// protect creates dir if needed and replaces its permissions with access, inherited by what
// is made in it and by what is there already.
func protect(dir, access string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	descriptor, err := windows.SecurityDescriptorFromString(access)
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(absolute, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
