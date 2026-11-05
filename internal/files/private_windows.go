//go:build windows

package files

import (
	"os"

	"golang.org/x/sys/windows"
)

// privateAccess is the file's owner, SYSTEM and Administrators, and nothing inherited.
const privateAccess = "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;OW)"

// keepPrivate sets the file's permissions by its name: the handle os.CreateTemp gives cannot
// change them, but its owner always may.
func keepPrivate(file *os.File) error {
	descriptor, err := windows.SecurityDescriptorFromString(privateAccess)
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(file.Name(), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// syncDirectory has nothing to do on Windows, which cannot open a directory to sync it:
// NTFS and ReFS journal the rename themselves.
func syncDirectory(string) error { return nil }
