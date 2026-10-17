package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// clock is a time that moves only when a test moves it.
type clock struct{ now time.Time }

func (moving *clock) read() time.Time { return moving.now }

func newLog(t *testing.T, rotateBytes int64, keep time.Duration) (*Log, *clock) {
	t.Helper()
	auditLog, err := New(t.TempDir(), rotateBytes, keep)
	if err != nil {
		t.Fatal(err)
	}
	moving := &clock{now: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)}
	auditLog.now = moving.read
	return auditLog, moving
}

func published(url, branch string) *gitdv1.AuditEntry {
	return &gitdv1.AuditEntry{
		Workspace:  "/var/lib/gitd/workspaces/agent-1",
		Url:        url,
		Branch:     branch,
		Repository: &gitdv1.PublishedRepository{Url: url, Commit: "abc123", Subjects: []string{"Fix the login"}},
	}
}

func TestEveryEntryIsReadBackNewestFirst(t *testing.T) {
	auditLog, moving := newLog(t, 1<<20, time.Hour)
	for _, branch := range []string{"agents/one", "agents/two", "agents/three"} {
		if err := auditLog.Record(published("https://github.com/acme/web", branch)); err != nil {
			t.Fatal(err)
		}
		moving.now = moving.now.Add(time.Minute)
	}
	entries, err := auditLog.Read(Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].GetBranch() != "agents/three" || entries[2].GetBranch() != "agents/one" {
		t.Fatalf("read %v, want the three newest first", entries)
	}
	if got := entries[2].GetTime().AsTime(); !got.Equal(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("the first entry is stamped %v", got)
	}
}

func TestAReadKeepsOnlyWhatTheFilterAsksFor(t *testing.T) {
	auditLog, moving := newLog(t, 1<<20, time.Hour)
	for _, url := range []string{"https://github.com/acme/web", "https://github.com/acme/api", "https://github.com/acme/web"} {
		if err := auditLog.Record(published(url, "agents/x")); err != nil {
			t.Fatal(err)
		}
		moving.now = moving.now.Add(time.Hour)
	}
	byURL, _ := auditLog.Read(Filter{URL: "https://github.com/acme/web", Limit: 10})
	if len(byURL) != 2 {
		t.Fatalf("by url: %d entries, want 2", len(byURL))
	}
	recent, _ := auditLog.Read(Filter{Since: time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC), Limit: 10})
	if len(recent) != 2 {
		t.Fatalf("since 10:00: %d entries, want 2", len(recent))
	}
	limited, _ := auditLog.Read(Filter{Limit: 1})
	if len(limited) != 1 || limited[0].GetUrl() != "https://github.com/acme/web" {
		t.Fatalf("limited: %v, want the newest only", limited)
	}
}

func TestTheFileIsRotatedAtItsSizeAndReadAcrossRotations(t *testing.T) {
	auditLog, moving := newLog(t, 300, 24*time.Hour)
	for index := range 6 {
		entry := published("https://github.com/acme/web", "agents/x")
		entry.Branch = "agents/" + string(rune('a'+index))
		if err := auditLog.Record(entry); err != nil {
			t.Fatal(err)
		}
		moving.now = moving.now.Add(time.Second)
	}
	rotated, _ := auditLog.rotated()
	if len(rotated) == 0 {
		t.Fatal("nothing was rotated")
	}
	for _, path := range append(rotated, auditLog.current()) {
		if info, _ := os.Stat(path); info.Size() > 300 {
			t.Fatalf("%s is %d bytes, past the rotation size", filepath.Base(path), info.Size())
		}
	}
	entries, _ := auditLog.Read(Filter{Limit: 10})
	if len(entries) != 6 || entries[0].GetBranch() != "agents/f" || entries[5].GetBranch() != "agents/a" {
		t.Fatalf("read %d entries across rotations, newest %q", len(entries), entries[0].GetBranch())
	}
}

func TestRotatedFilesOlderThanKeepAreDeleted(t *testing.T) {
	auditLog, moving := newLog(t, 1, time.Hour)
	old := filepath.Join(auditLog.directory, rotatedPrefix+"20260101T000000.000000000Z"+rotatedSuffix)
	if err := os.WriteFile(old, []byte("{}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(old, moving.now.Add(-2*time.Hour), moving.now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := auditLog.Record(published("https://github.com/acme/web", "agents/x")); err != nil {
			t.Fatal(err)
		}
		moving.now = moving.now.Add(time.Second)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("a rotated file older than keep is still there")
	}
}

func TestAHalfWrittenLineIsSkippedAndTheNextEntryKept(t *testing.T) {
	auditLog, _ := newLog(t, 1<<20, time.Hour)
	if err := auditLog.Record(published("https://github.com/acme/web", "agents/x")); err != nil {
		t.Fatal(err)
	}
	file, _ := os.OpenFile(auditLog.current(), os.O_APPEND|os.O_WRONLY, 0)
	_, _ = file.WriteString(`{"workspace":"/torn`)
	_ = file.Close()
	if err := auditLog.Record(published("https://github.com/acme/web", "agents/y")); err != nil {
		t.Fatal(err)
	}
	entries, err := auditLog.Read(Filter{Limit: 10})
	if err != nil || len(entries) != 2 || entries[0].GetBranch() != "agents/y" {
		t.Fatalf("read %d entries (%v), want the two whole ones", len(entries), err)
	}
}

func TestAnEntryKeepsTheTimeItCameWith(t *testing.T) {
	auditLog, _ := newLog(t, 1<<20, time.Hour)
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	entry := published("https://github.com/acme/web", "agents/x")
	entry.Time = timestamppb.New(when)
	if err := auditLog.Record(entry); err != nil {
		t.Fatal(err)
	}
	entries, _ := auditLog.Read(Filter{Limit: 1})
	if !entries[0].GetTime().AsTime().Equal(when) {
		t.Fatalf("time %v, want %v", entries[0].GetTime().AsTime(), when)
	}
}
