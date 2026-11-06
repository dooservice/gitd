package strategy

import (
	"context"
	"io/fs"
	"sync"
)

// cloner clones files with a fixed number of workers and keeps the first failure, after
// which the files still to come are not cloned.
type cloner struct {
	ctx     context.Context
	jobs    chan cloneJob
	workers sync.WaitGroup
	guard   sync.Mutex
	err     error
}

type cloneJob struct {
	source, target string
	info           fs.FileInfo
}

func newCloner(ctx context.Context, workers int) *cloner {
	files := &cloner{ctx: ctx, jobs: make(chan cloneJob, workers)}
	for range max(workers, 1) {
		files.workers.Add(1)
		go func() {
			defer files.workers.Done()
			for job := range files.jobs {
				if files.failed() != nil {
					continue
				}
				if err := clone(job.source, job.target, job.info); err != nil {
					files.fail(err)
				}
			}
		}()
	}
	return files
}

func (files *cloner) add(source, target string, info fs.FileInfo) {
	files.jobs <- cloneJob{source: source, target: target, info: info}
}

// failed is the first failure, or the context's end.
func (files *cloner) failed() error {
	files.guard.Lock()
	defer files.guard.Unlock()
	if files.err == nil {
		files.err = files.ctx.Err()
	}
	return files.err
}

func (files *cloner) fail(err error) {
	files.guard.Lock()
	defer files.guard.Unlock()
	if files.err == nil {
		files.err = err
	}
}

// wait waits for every file added, and says the first failure.
func (files *cloner) wait() error {
	close(files.jobs)
	files.workers.Wait()
	return files.failed()
}
