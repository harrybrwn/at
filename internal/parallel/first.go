package parallel

import (
	"context"
	"sync"
)

func FirstOf[I, O any](
	ctx context.Context,
	in I,
	jobs ...Job[I, O],
) (O, error) {
	ctrl := NewCtrl[I, O](nil)
	return ctrl.FirstOf(ctx, in, jobs)
}

// FirstOf takes an input and a list of jobs and returns the result of the job
// that finishes first.
func (c *Ctrl[In, Out]) FirstOf(
	ctx context.Context,
	in In,
	jobs []Job[In, Out],
) (Out, error) {
	type payload struct {
		o   Out
		err error
	}
	ctx, cancel := c.cfg.context(ctx)
	ch := make(chan payload)
	defer cancel()

	// Wait for all jobs to finish before closing the channel
	var wg sync.WaitGroup
	wg.Add(len(jobs))
	go func() {
		wg.Wait()
		close(ch)
	}()

	for _, job := range jobs {
		go func(fn Job[In, Out]) {
			defer wg.Done()
			o, err := fn(ctx, in)
			select {
			case <-ctx.Done():
			case ch <- payload{o, err}:
			}
		}(job)
	}

	var err error
	for {
		select {
		case <-ctx.Done():
			var zero Out
			if err == nil {
				err = ctx.Err()
			}
			return zero, err
		case p := <-ch:
			if p.err != nil {
				if err == nil {
					err = p.err
				}
				continue
			}
			return p.o, nil
		}
	}
}
