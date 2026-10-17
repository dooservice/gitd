// Package audit keeps the audit log: every publish, done or refused, one per line of an
// append-only file, in the JSON form of the contract's AuditEntry. The file is rotated when
// it reaches a size; rotated files are deleted once older than an age.
package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// Log is the audit log in a directory: the file being written, and those rotated out of it.
// RotateBytes is the size the file is rotated at; Keep is how long a rotated file is kept.
type Log struct {
	directory   string
	rotateBytes int64
	keep        time.Duration
	now         func() time.Time
	writing     sync.Mutex
}

// New returns the audit log in directory, making it if it is not there, and deletes the
// rotated files older than keep.
func New(directory string, rotateBytes int64, keep time.Duration) (*Log, error) {
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, fmt.Errorf("audit log: %w", err)
	}
	auditLog := &Log{directory: directory, rotateBytes: rotateBytes, keep: keep, now: time.Now}
	if err := auditLog.prune(); err != nil {
		return nil, err
	}
	return auditLog, nil
}

// Record appends an entry, stamped with the time now unless it has one, and syncs it to
// disk before it returns.
func (auditLog *Log) Record(entry *gitdv1.AuditEntry) error {
	auditLog.writing.Lock()
	defer auditLog.writing.Unlock()
	if entry.GetTime() == nil {
		entry.Time = timestamppb.New(auditLog.now())
	}
	line, err := protojson.Marshal(entry)
	if err != nil {
		return fmt.Errorf("audit log: %w", err)
	}
	line = append(line, '\n')
	if err := auditLog.rotateBefore(int64(len(line))); err != nil {
		return err
	}
	file, err := os.OpenFile(auditLog.current(), os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o640)
	if err != nil {
		return fmt.Errorf("audit log: %w", err)
	}
	if torn, err := endsTorn(file); err != nil || torn {
		// What a crash left half written stays a line of its own, which a read skips.
		line = append([]byte{'\n'}, line...)
	}
	if _, err := file.Write(line); err != nil {
		_ = file.Close()
		return fmt.Errorf("audit log: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("audit log: %w", err)
	}
	return file.Close()
}

// current is the file being written.
func (auditLog *Log) current() string {
	return filepath.Join(auditLog.directory, currentFile)
}

// endsTorn says whether the file's last line is not finished.
func endsTorn(file *os.File) (bool, error) {
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return false, err
	}
	last := make([]byte, 1)
	if _, err := file.ReadAt(last, info.Size()-1); err != nil {
		return false, err
	}
	return last[0] != '\n', nil
}
