package audit

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// Filter is which entries a read returns: from Since on (all when zero), of the
// repository at URL (any when empty), at most Limit of them.
type Filter struct {
	Since time.Time
	URL   string
	Limit int
}

// maxLine is the longest line read back: far more than any entry, so a damaged file
// cannot make a read hold it all.
const maxLine = 1 << 20

// Read returns the entries the filter keeps, newest first. A line that is not an entry,
// as a crash in the middle of a write may leave, is skipped.
func (auditLog *Log) Read(filter Filter) ([]*gitdv1.AuditEntry, error) {
	rotated, err := auditLog.rotated()
	if err != nil {
		return nil, err
	}
	files := append(rotated, auditLog.current())
	var kept []*gitdv1.AuditEntry
	for index := len(files) - 1; index >= 0 && len(kept) < filter.Limit; index-- {
		entries, err := readFile(files[index])
		if err != nil {
			return nil, err
		}
		for position := len(entries) - 1; position >= 0 && len(kept) < filter.Limit; position-- {
			entry := entries[position]
			if !filter.Since.IsZero() && entry.GetTime().AsTime().Before(filter.Since) {
				return kept, nil
			}
			if filter.URL == "" || entry.GetUrl() == filter.URL {
				kept = append(kept, entry)
			}
		}
	}
	return kept, nil
}

// readFile is the entries of one file, oldest first; a file not there has none.
func readFile(path string) ([]*gitdv1.AuditEntry, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("audit log: %w", err)
	}
	defer file.Close()
	var entries []*gitdv1.AuditEntry
	lines := bufio.NewScanner(file)
	lines.Buffer(make([]byte, 0, 64*1024), maxLine)
	for number := 1; lines.Scan(); number++ {
		entry := &gitdv1.AuditEntry{}
		if err := protojson.Unmarshal(lines.Bytes(), entry); err != nil {
			slog.Warn("audit log: a line is not an entry", "file", path, "line", number, "error", err)
			continue
		}
		entries = append(entries, entry)
	}
	if err := lines.Err(); err != nil {
		return nil, fmt.Errorf("audit log: %s: %w", path, err)
	}
	return entries, nil
}
