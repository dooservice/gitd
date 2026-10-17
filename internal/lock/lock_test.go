package lock

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAWaitTheCallerGaveUpOnIsNotBusy(t *testing.T) {
	locks, err := NewLocks(t.TempDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	release, err := locks.Acquire(context.Background(), "pool:x")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := locks.Acquire(cancelled, "pool:x"); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled wait = %v", err)
	}
	short, err := NewLocks(t.TempDir(), 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	held, _ := short.Acquire(context.Background(), "pool:y")
	defer held()
	if _, err := short.Acquire(context.Background(), "pool:y"); !errors.Is(err, ErrBusy) {
		t.Errorf("wait past maxWait = %v", err)
	}
}

func TestAFullQueueRefusesAndAReleaseIsOnce(t *testing.T) {
	queue := NewQueue(1, 0, 10*time.Millisecond)
	leave, err := queue.Enter(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Enter(context.Background()); !errors.Is(err, ErrBusy) {
		t.Errorf("full queue = %v", err)
	}
	leave()
	leave()
	again, err := queue.Enter(context.Background())
	if err != nil {
		t.Fatalf("after leaving: %v", err)
	}
	again()
}
