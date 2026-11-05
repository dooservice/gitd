//go:build windows

package strategy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// preferReflink is whether, without an overlay, gitd tries to clone templates: on a Dev
// Drive, or any ReFS volume, Windows clones a file's blocks instead of copying them.
const preferReflink = true

const (
	fsctlDuplicateExtentsToFile  = 0x00098344
	fsctlGetIntegrityInformation = 0x0009027C
	fsctlSetIntegrityInformation = 0x0009C280
	fsctlSetSparse               = 0x000900C4
	duplicateChunk               = int64(1 << 30)
)

// duplicateExtentsData is DUPLICATE_EXTENTS_DATA: which part of which file to clone where.
type duplicateExtentsData struct {
	FileHandle       windows.Handle
	SourceFileOffset int64
	TargetFileOffset int64
	ByteCount        int64
}

// integrityInformation is FSCTL_GET_INTEGRITY_INFORMATION_BUFFER: only ReFS answers it.
type integrityInformation struct {
	ChecksumAlgorithm        uint16
	Reserved                 uint16
	Flags                    uint32
	ChecksumChunkSizeInBytes uint32
	ClusterSizeInBytes       uint32
}

// setIntegrityInformation is FSCTL_SET_INTEGRITY_INFORMATION_BUFFER.
type setIntegrityInformation struct {
	ChecksumAlgorithm uint16
	Reserved          uint16
	Flags             uint32
}

// MountOverlay is unsupported: Windows has no overlay.
func (Kernel) MountOverlay([]string, string, string, string) error {
	return fmt.Errorf("%w: Windows has no overlay", ErrUnsupported)
}

// Unmount has nothing to unmount on Windows.
func (Kernel) Unmount(string) error { return nil }

func mountedOverlay(string) bool { return false }

func onOverlay(string) bool { return false }

func mountedFilesystem(string) (uint64, bool) { return 0, false }

func heldElsewhere(uint64) bool { return false }

// clone makes target share source's blocks, with source's mode and modification time. Block
// cloning needs both files on one ReFS volume, the target as long as the source and with the
// same integrity setting, and whole clusters: the last one is rounded up, which the target's
// length then cuts back.
func clone(source, target string, info fs.FileInfo) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	integrity, err := integrityOf(windows.Handle(in.Fd()))
	if err != nil {
		return fmt.Errorf("%w: %s is not on ReFS: %v", ErrUnsupported, source, err)
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if err := duplicate(windows.Handle(in.Fd()), out, info.Size(), integrity); err != nil {
		_ = out.Close()
		_ = os.Remove(target)
		if errors.Is(err, windows.ERROR_NOT_SUPPORTED) || errors.Is(err, windows.ERROR_INVALID_FUNCTION) || errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) {
			return fmt.Errorf("%w: %v", ErrUnsupported, err)
		}
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(target, info.ModTime(), info.ModTime())
}

func duplicate(source windows.Handle, out *os.File, size int64, integrity integrityInformation) error {
	target := windows.Handle(out.Fd())
	setting := setIntegrityInformation{ChecksumAlgorithm: integrity.ChecksumAlgorithm, Flags: integrity.Flags}
	var returned uint32
	if err := windows.DeviceIoControl(target, fsctlSetIntegrityInformation, (*byte)(unsafe.Pointer(&setting)), uint32(unsafe.Sizeof(setting)), nil, 0, &returned, nil); err != nil {
		return err
	}
	if sparse(source) {
		if err := windows.DeviceIoControl(target, fsctlSetSparse, nil, 0, nil, 0, &returned, nil); err != nil {
			return err
		}
	}
	if err := out.Truncate(size); err != nil {
		return err
	}
	cluster := int64(integrity.ClusterSizeInBytes)
	for offset := int64(0); offset < size; offset += duplicateChunk {
		count := min(duplicateChunk, size-offset)
		count = (count + cluster - 1) / cluster * cluster
		extents := duplicateExtentsData{FileHandle: source, SourceFileOffset: offset, TargetFileOffset: offset, ByteCount: count}
		if err := windows.DeviceIoControl(target, fsctlDuplicateExtentsToFile, (*byte)(unsafe.Pointer(&extents)), uint32(unsafe.Sizeof(extents)), nil, 0, &returned, nil); err != nil {
			return err
		}
	}
	return nil
}

// integrityOf is the integrity setting and cluster size of an open file's volume; a volume
// that is not ReFS cannot answer, and cannot clone blocks either.
func integrityOf(handle windows.Handle) (integrityInformation, error) {
	var information integrityInformation
	var returned uint32
	err := windows.DeviceIoControl(handle, fsctlGetIntegrityInformation, nil, 0, (*byte)(unsafe.Pointer(&information)), uint32(unsafe.Sizeof(information)), &returned, nil)
	if err == nil && information.ClusterSizeInBytes == 0 {
		err = errors.New("the volume has no cluster size")
	}
	return information, err
}

func sparse(handle windows.Handle) bool {
	var info windows.ByHandleFileInformation
	return windows.GetFileInformationByHandle(handle, &info) == nil && info.FileAttributes&windows.FILE_ATTRIBUTE_SPARSE_FILE != 0
}

// cloneTree clones a whole template at once where the system can; on Windows every file is
// cloned on its own.
func cloneTree(string, string) (bool, error) {
	return false, nil
}
