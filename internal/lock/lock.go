// Package lock serializes the writes on one repository and bounds how many run at once.
//
// A lock is held in memory and backed by an flock on disk, so a second gitd, or one that
// restarted while the first one's git was still running, waits too. Both are taken before
// git takes its own `.lock` files, which is what lets a stale one be told from a live one.
package lock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// ErrBusy is a wait that ran out of time or room.
var ErrBusy = errors.New("busy")

// Locks are the locks of one gitd, one per key, each waited for at most maxWait.
type Locks struct {
	dir     string
	maxWait time.Duration
	guard   sync.Mutex
	held    map[string]chan struct{}
}

// NewLocks keeps the lock files in dir.
func NewLocks(dir string, maxWait time.Duration) (*Locks, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Locks{dir: dir, maxWait: maxWait, held: map[string]chan struct{}{}}, nil
}

// Acquire waits for key, and returns what releases it.
func (locks *Locks) Acquire(ctx context.Context, key string) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, locks.maxWait)
	defer cancel()
	slot := locks.slot(key)
	select {
	case slot <- struct{}{}:
	case <-ctx.Done():
		return nil, ErrBusy
	}
	file, err := locks.flock(ctx, key)
	if err != nil {
		<-slot
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			_ = file.Close()
			<-slot
		})
	}, nil
}

func (locks *Locks) slot(key string) chan struct{} {
	locks.guard.Lock()
	defer locks.guard.Unlock()
	slot, ok := locks.held[key]
	if !ok {
		slot = make(chan struct{}, 1)
		locks.held[key] = slot
	}
	return slot
}

// flock takes the file lock of key, trying again until ctx ends: flock itself cannot be
// told to stop waiting.
func (locks *Locks) flock(ctx context.Context, key string) (*os.File, error) {
	sum := sha256.Sum256([]byte(key))
	file, err := os.OpenFile(filepath.Join(locks.dir, hex.EncodeToString(sum[:12])+".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = file.Close()
			return nil, err
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			_ = file.Close()
			return nil, ErrBusy
		}
	}
}
