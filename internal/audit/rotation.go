package audit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The file being written is publishes.jsonl; a rotated one is named after when it was
// rotated, publishes-20261017T072100.000000000Z.jsonl, so names sort oldest first.
const (
	currentFile   = "publishes.jsonl"
	rotatedPrefix = "publishes-"
	rotatedSuffix = ".jsonl"
	rotatedStamp  = "20060102T150405.000000000Z"
)

// rotateBefore rotates the file being written when adding size bytes would take it past
// rotateBytes, and then deletes the rotated files too old to keep.
func (auditLog *Log) rotateBefore(size int64) error {
	info, err := os.Stat(auditLog.current())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("audit log: %w", err)
	}
	if info.Size() == 0 || info.Size()+size <= auditLog.rotateBytes {
		return nil
	}
	rotated := filepath.Join(auditLog.directory, rotatedPrefix+auditLog.now().UTC().Format(rotatedStamp)+rotatedSuffix)
	if err := os.Rename(auditLog.current(), rotated); err != nil {
		return fmt.Errorf("audit log: %w", err)
	}
	return auditLog.prune()
}

// prune deletes the rotated files last written longer ago than keep.
func (auditLog *Log) prune() error {
	rotated, err := auditLog.rotated()
	if err != nil {
		return err
	}
	oldest := auditLog.now().Add(-auditLog.keep)
	for _, path := range rotated {
		info, err := os.Stat(path)
		if err != nil || !info.ModTime().Before(oldest) {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("audit log: %w", err)
		}
	}
	return nil
}

// rotated is the rotated files, oldest first.
func (auditLog *Log) rotated() ([]string, error) {
	listed, err := os.ReadDir(auditLog.directory)
	if err != nil {
		return nil, fmt.Errorf("audit log: %w", err)
	}
	var paths []string
	for _, entry := range listed {
		name := entry.Name()
		if entry.Type().IsRegular() && strings.HasPrefix(name, rotatedPrefix) && strings.HasSuffix(name, rotatedSuffix) {
			paths = append(paths, filepath.Join(auditLog.directory, name))
		}
	}
	sort.Strings(paths)
	return paths, nil
}
