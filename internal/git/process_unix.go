//go:build !windows

package git

import (
	"os/exec"
	"syscall"
)

// processGroup is git and the children it starts: git leads a process group of its own, and
// cancelling kills the group.
type processGroup struct{}

func newProcessGroup(process *exec.Cmd) (*processGroup, error) {
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	process.Cancel = func() error {
		return syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
	}
	return &processGroup{}, nil
}

func (*processGroup) run(process *exec.Cmd) error {
	return process.Run()
}

func (*processGroup) close() {}
