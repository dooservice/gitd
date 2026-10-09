package lock

import (
	"context"
	"sync"
	"time"
)

// Queue bounds how many operations of one kind run at once and how many wait, each for at
// most maxWait. What does not fit is refused rather than left to pile up.
type Queue struct {
	slots   chan struct{}
	waiting chan struct{}
	maxWait time.Duration
}

// NewQueue lets running operations run and queued more wait.
func NewQueue(running, queued int, maxWait time.Duration) *Queue {
	return &Queue{
		slots:   make(chan struct{}, running),
		waiting: make(chan struct{}, running+queued),
		maxWait: maxWait,
	}
}

// Enter waits for a slot and returns what leaves it, or ErrBusy when the queue is full or
// the wait is too long.
func (queue *Queue) Enter(ctx context.Context) (func(), error) {
	select {
	case queue.waiting <- struct{}{}:
	default:
		return nil, ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, queue.maxWait)
	defer cancel()
	select {
	case queue.slots <- struct{}{}:
	case <-ctx.Done():
		<-queue.waiting
		return nil, ErrBusy
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			<-queue.slots
			<-queue.waiting
		})
	}, nil
}
