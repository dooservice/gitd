package strategy

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// mountedFilesystem is the device number of the filesystem mounted at dir, and whether
// one is: a directory on the same device as its parent is not a mount.
func mountedFilesystem(dir string) (uint64, bool) {
	var own, parent unix.Stat_t
	if unix.Stat(dir, &own) != nil || unix.Stat(filepath.Dir(dir), &parent) != nil {
		return 0, false
	}
	return own.Dev, own.Dev != parent.Dev
}

// heldElsewhere says whether any process gitd can see still has the filesystem numbered
// dev mounted: a sandbox's bind of a workspace lives in the sandbox's own mount namespace,
// and unmounting the workspace leaves it, and its writes, alive.
func heldElsewhere(dev uint64) bool {
	wanted := fmt.Sprintf("%d:%d", unix.Major(dev), unix.Minor(dev))
	processes, _ := filepath.Glob("/proc/[0-9]*/mountinfo")
	for _, mountinfo := range processes {
		if mounts(mountinfo, wanted) {
			return true
		}
	}
	return false
}

// mounts says whether the mountinfo file lists a mount of the device numbered wanted,
// written major:minor, its third field.
func mounts(mountinfo, wanted string) bool {
	file, err := os.Open(mountinfo)
	if err != nil {
		return false
	}
	defer file.Close()
	lines := bufio.NewScanner(file)
	lines.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for lines.Scan() {
		fields := strings.Fields(lines.Text())
		if len(fields) > 2 && fields[2] == wanted {
			return true
		}
	}
	return false
}
