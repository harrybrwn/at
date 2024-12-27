package parallel

import (
	"context"
	"log/slog"
	"time"
)

type JobConfig struct {
	Timeout time.Duration
	Logger  *slog.Logger
}

func (jc *JobConfig) defaults() {
	if jc.Logger == nil {
		jc.Logger = slog.Default()
	}
}

func (jc *JobConfig) context(ctx context.Context) (context.Context, context.CancelFunc) {
	cctx, cancel := context.WithCancel(ctx)
	if jc.Timeout > 0 {
		tctx, timeout := context.WithTimeout(cctx, jc.Timeout)
		return tctx, func() {
			timeout()
			cancel()
		}
	}
	return cctx, cancel
}
