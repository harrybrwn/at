package parallel

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestMap(t *testing.T) {
	ctx := t.Context()
	in := []string{"a", "b", "c", "d", "e"}
	results, err := Map(ctx, in, func(ctx context.Context, v string) (string, error) {
		return "_" + v, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(in) {
		t.Fatal("results are a different length from the input")
	}
	for i, v := range in {
		if results[i] != "_"+v {
			t.Errorf("expected %q, got %q", "_"+v, results[i])
		}
	}
}

func TestMapCancellation(t *testing.T) {
	var (
		n  atomic.Int32
		in = make([]int, 60)
	)
	ctx, cancel := context.WithCancel(t.Context())
	for i := range in {
		in[i] = i
	}
	_ = cancel
	_, err := Map(ctx, in, func(_ context.Context, v int) (int, error) {
		n.Add(1)
		if v == 20 {
			cancel()
		}
		time.Sleep(time.Millisecond * 10)
		return v, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context cancelled error, got %v", err)
	}
	if int(n.Load()) == len(in) {
		t.Error("shouldn't have executed all the jobs")
	}

	ctx, cancel = context.WithCancel(t.Context())
	_, err = Map(ctx, in, func(context.Context, int) (int, error) {
		cancel()
		return 0, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context cancelled error, got %v", err)
	}
}

func TestFirstOf(t *testing.T) {
	job := func(n int, e error) Job[int, int] {
		return func(ctx context.Context, in int) (int, error) {
			time.Sleep(time.Millisecond * time.Duration(n))
			return n, e
		}
	}
	t.Run("SkipsErrors", func(t *testing.T) {
		ctx := t.Context()
		start := time.Now()
		res, err := FirstOf(
			ctx, 0,
			job(100, nil),
			job(2, errors.New("test error")), // cannot be 2 even if its first because there is an error
			job(250, nil),
			job(1000, nil),
			job(10, nil),
		)
		if err != nil {
			t.Fatal(err)
		}
		if res != 10 {
			t.Errorf("expected 5, got %d", res)
		}
		checkBetween(t, time.Since(start), 9*time.Millisecond, 11*time.Millisecond)
	})

	t.Run("Timeout", func(t *testing.T) {
		ctx := t.Context()
		ctrl := NewCtrl[int, int](&JobConfig{Timeout: time.Millisecond * 10})
		_, err := ctrl.FirstOf(
			ctx,
			0,
			[]Job[int, int]{
				job(1e4, nil),
				job(1e4, nil),
				job(1e4, nil),
			},
		)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context deadline exceeded error, got \"%v\"", err)
		}
	})

	t.Run("Canceled", func(t *testing.T) {
		ctx := t.Context()
		start := time.Now()
		ctx, cancel := context.WithCancel(ctx)
		var jobs Jobs[int, int]
		jobs.Add(
			job(10000, nil),
			job(10001, nil),
			job(10002, nil),
			job(10003, nil),
		)
		jobs.Add(func(context.Context, int) (int, error) {
			cancel()
			return 0, nil
		})
		_, err := FirstOf(ctx, 0, jobs...)
		if !errors.Is(err, context.Canceled) {
			t.Error("expected context canceled error")
		}
		if time.Since(start) > time.Millisecond*10 {
			t.Error("FirstOf jobs took too long after cancellation")
		}
	})
}

func TestMap_Err(t *testing.T) {
	errTest := errors.New("test error")
	var ran5 atomic.Int32
	ctx := t.Context()
	in := []int{1, 2, 3, 4, 5}
	res, err := NewCtrl[int, string](&JobConfig{Timeout: time.Minute}).Map(ctx, in, func(_ context.Context, v int) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		if v == 2 {
			return "", errTest
		}
		time.Sleep(time.Millisecond * 1000)
		if v == 5 {
			ran5.Add(1)
		}
		return fmt.Sprintf("%d", v), nil
	})
	// res, err := Map(ctx, &JobConfig{Timeout: time.Minute}, in, func(_ context.Context, v int) (string, error) {
	// 	select {
	// 	case <-ctx.Done():
	// 		return "", ctx.Err()
	// 	default:
	// 	}
	// 	if v == 2 {
	// 		return "", errTest
	// 	}
	// 	time.Sleep(time.Millisecond * 1000)
	// 	if v == 5 {
	// 		ran5.Add(1)
	// 	}
	// 	return fmt.Sprintf("%d", v), nil
	// })
	if !errors.Is(err, errTest) {
		t.Errorf("expected the error %v, got %v", errTest, err)
	}
	if ran5.Load() > 0 {
		t.Error("should not have run job number 5")
	}
	_ = res
}

func TestDo(t *testing.T) {
	errTest := errors.New("test error")
	ctx := t.Context()
	var n atomic.Int32
	j := func(context.Context) error {
		n.Add(1)
		return nil
	}
	var jobs BasicJobs
	jobs.Add(j, j, j, j, j, j, j, j, j, j)
	err := Do(
		ctx,
		jobs...,
	)
	if err != nil {
		t.Fatal(err)
	}
	if n.Load() != 10 {
		t.Error("expected all 10 jobs to run")
	}

	n.Store(0)
	err = Do(
		ctx,
		j, j, j, j, j,
		func(ctx context.Context) error {
			n.Add(1)
			return errTest
		},
		j, j, j, j, j,
		func(ctx context.Context) error {
			time.Sleep(time.Millisecond * 250)
			n.Add(1)
			return nil
		},
	)
	if !errors.Is(err, errTest) {
		t.Errorf("expected the error %v, got %v", errTest, err)
	}
	if n.Load() >= 12 {
		t.Errorf("not all 12 jobs should have run: %d jobs ran", n.Load())
	}
}

func TestDo_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Do(ctx, func(ctx context.Context) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context canceled error, got \"%v\"", err)
	}
}

func checkBetween(t *testing.T, took, start, end time.Duration) {
	t.Helper()
	if end-start < 10*time.Millisecond {
		buffer := time.Nanosecond * 1000
		start -= buffer
		end += buffer
	}
	if took < start || took > end {
		t.Errorf("expected %v to be between %v and %v", took, start, end)
	}
}
