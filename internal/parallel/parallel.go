package parallel

import (
	"context"
	"sync"
)

type Job[I, O any] func(ctx context.Context, in I) (O, error)

type Jobs[I, O any] []Job[I, O]

func (jobs *Jobs[I, O]) Add(j ...Job[I, O]) {
	*jobs = append(*jobs, j...)
}

type BasicJobs []BasicJob

func (jobs *BasicJobs) Add(j ...BasicJob) {
	*jobs = append(*jobs, j...)
}

type BasicJob func(context.Context) error

type Ctrl[In, Out any] struct {
	cfg JobConfig
	_   In
	_   Out
}

func NewCtrl[I, O any](config *JobConfig) *Ctrl[I, O] {
	if config == nil {
		config = new(JobConfig)
	}
	config.defaults()
	return &Ctrl[I, O]{cfg: *config}
}

// Do will execute a list of jobs in parallel.
func Do(ctx context.Context, jobs ...BasicJob) error {
	ctrl := NewCtrl[any, any](nil)
	return ctrl.Do(ctx, jobs)
}

// Do will execute a list of jobs in parallel.
func (c *Ctrl[In, Out]) Do(ctx context.Context, jobs []BasicJob) error {
	ctx, cancel := c.cfg.context(ctx)
	defer cancel()
	var (
		wg sync.WaitGroup
		ch = make(chan error)
	)
	wg.Add(len(jobs))
	go func() {
		wg.Wait()
		close(ch)
	}()
	for _, job := range jobs {
		go func(fn BasicJob) {
			defer wg.Done()
			err := fn(ctx)
			select {
			case <-ctx.Done():
			case ch <- err:
			}
		}(job)
	}

	for e := range ch {
		if e != nil {
			return e
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return nil
}
