//go:build windows

package server

import (
	"golang.org/x/sys/windows"

	"github.com/dooservice/gitd/internal/winservice"
)

// socketAccess lets only the socket's owner, SYSTEM, Administrators and the group gitd, when
// there is one, open it, which connecting to a Unix socket on Windows needs; nothing is
// inherited from the directory.
func socketAccess() string {
	access := "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;OW)"
	if group, ok := winservice.GroupSID(); ok {
		access += "(A;;FA;;;" + group.String() + ")"
	}
	return access
}

// restrictSocket replaces the socket's permissions with socketAccess. The socket is opened
// as itself, not followed, as Windows keeps it as a reparse point.
func restrictSocket(socket string) error {
	descriptor, err := windows.SecurityDescriptorFromString(socketAccess())
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(socket)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(name, windows.WRITE_DAC|windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
