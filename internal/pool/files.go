package pool

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Temp is a fresh temporary name next to path.
//
// Everything gitd builds is built under such a name and moved into place when it is
// complete. Whatever still carries one after a crash is half built, and the next start
// removes it.
func Temp(path string) string {
	random := make([]byte, 6)
	_, _ = rand.Read(random)
	return path + ".gitd-tmp-" + hex.EncodeToString(random)
}

// IsTemp is whether a name is one Temp gave.
func IsTemp(name string) bool {
	return strings.Contains(name, ".gitd-tmp-")
}

// WriteAtomic writes a file so that it is either the old one or the new one, never half.
func WriteAtomic(path string, data []byte) error {
	temp := Temp(path)
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// All are the directories of every pool, found on disk.
func (pools *Pools) All() []string {
	found, _ := filepath.Glob(filepath.Join(pools.Dir, "*", "*", "*.git"))
	return slices.DeleteFunc(found, func(dir string) bool { return IsTemp(filepath.Base(dir)) })
}
