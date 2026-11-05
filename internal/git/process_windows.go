//go:build windows

package git

import (
	"fmt"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processGroup is git and the children it starts, held in a job object: cancelling ends the
// job, and closing it ends whatever is still in it.
type processGroup struct {
	job windows.Handle
}

func newProcessGroup(process *exec.Cmd) (*processGroup, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	process.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	process.Cancel = func() error {
		return windows.TerminateJobObject(job, 1)
	}
	return &processGroup{job: job}, nil
}

// run starts git suspended, puts it in the job and only then lets it run, so no child it
// starts can escape the job.
func (group *processGroup) run(process *exec.Cmd) error {
	if err := process.Start(); err != nil {
		return err
	}
	if err := group.adopt(uint32(process.Process.Pid)); err != nil {
		_ = process.Process.Kill()
		_ = process.Wait()
		return err
	}
	return process.Wait()
}

func (group *processGroup) adopt(pid uint32) error {
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	if err := windows.AssignProcessToJobObject(group.job, handle); err != nil {
		return err
	}
	return resumeThreads(pid)
}

// resumeThreads resumes the threads of a process started suspended: it has only its first.
func resumeThreads(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := 0
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		_, err = windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if err != nil {
			return err
		}
		resumed++
	}
	if resumed == 0 {
		return fmt.Errorf("process %d has no thread to resume", pid)
	}
	return nil
}

func (group *processGroup) close() {
	_ = windows.CloseHandle(group.job)
}
