package parallel

import (
	"context"
	"sync"
)

// Map takes a list of inputs and applies a job to them all in parallel. If one
// job fails then the rest of the unfinished or incomplete jobs will be
// cancelled and may not finish.
func Map[In, Out any](ctx context.Context, elements []In, job Job[In, Out]) ([]Out, error) {
	ctrl := NewCtrl[In, Out](nil)
	return ctrl.Map(ctx, elements, job)
}

// Map takes a list of inputs and applies a job to them all in parallel. If one
// job fails then the rest of the unfinished or incomplete jobs will be
// cancelled and may not finish.
func (c *Ctrl[In, Out]) Map(
	ctx context.Context,
	elements []In,
	job Job[In, Out],
) ([]Out, error) {
	ctx, cancel := c.cfg.context(ctx)
	defer cancel()
	var (
		wg      sync.WaitGroup
		errs    = make(chan error)
		results = make([]Out, len(elements))
	)
	wg.Add(len(elements))
	go func() {
		wg.Wait()
		close(errs)
	}()

	for i := range elements {
		go func(i int, e In) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
			default:
				v, err := job(ctx, e)
				if err != nil {
					select {
					case <-ctx.Done():
					case errs <- err:
					}
				} else {
					results[i] = v
				}
			}
		}(i, elements[i])
	}

	for err := range errs {
		if err != nil {
			return results, err
		}
	}
	return results, nil
}
