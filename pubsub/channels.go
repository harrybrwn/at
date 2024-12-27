package pubsub

import (
	"context"
	"errors"
	"sync"

	"github.com/harrybrwn/at/array"
)

func NewMemoryBus[E any]() *ChannelBus[E] {
	return &ChannelBus[E]{queues: make([]queue[E], 0)}
}

type Empty struct{}

type ChannelBus[E any] struct {
	queues []queue[E]
	mu     sync.RWMutex
}

func (cb *ChannelBus[E]) Close() error {
	N := len(cb.queues)
	for i := 0; i < N; i++ {
		err := cb.queues[i].Close()
		if err != nil {
			return err
		}
	}
	if len(cb.queues) > 0 {
		return errors.New("failed to close all subscribers")
	}
	cb.queues = make([]queue[E], 0)
	return nil
}

func (cb *ChannelBus[E]) Subscriber(ctx context.Context, _ ...Empty) (Sub[E], error) {
	cb.mu.Lock()
	q := queue[E]{
		pub:  make(chan E),
		sub:  make(chan E),
		done: make(chan struct{}),
		ix:   int64(len(cb.queues)),
		bus:  cb,
	}
	cb.queues = append(cb.queues, q)
	cb.mu.Unlock()
	go start(ctx, &q)
	return &q, nil
}

func (cb *ChannelBus[E]) Publisher(ctx context.Context, _ ...Empty) (Pub[E], error) {
	return &channelBusPublisher[E]{cb: cb}, nil
}

type channelBusPublisher[E any] struct{ cb *ChannelBus[E] }

// Close does nothing since this should just publish to all existing channels.
func (cbp *channelBusPublisher[E]) Close() error {
	return nil
}

func (cbp *channelBusPublisher[E]) Pub(ctx context.Context, evt E) error {
	return cbp.cb.pub(ctx, evt)
}

func (cb *ChannelBus[E]) pub(ctx context.Context, evt E) error {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	for _, q := range cb.queues {
		err := q.Pub(ctx, evt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (cb *ChannelBus[E]) remove(q *queue[E]) {
	if q.ix < 0 {
		return
	}
	cb.mu.Lock()
	cb.queues = array.Remove(int(q.ix), cb.queues)
	cb.mu.Unlock()
	q.ix = -1
}

type queue[E any] struct {
	pub, sub chan E
	done     chan struct{}
	ix       int64
	bus      *ChannelBus[E]
}

func (q *queue[E]) Close() error {
	close(q.done)
	close(q.pub)
	if q.bus != nil {
		q.bus.remove(q)
	}
	return nil
}

func start[E any](ctx context.Context, q *queue[E]) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		close(q.sub)
		cancel()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-q.done:
			return
		case msg, ok := <-q.pub:
			if !ok {
				return
			}
			q.sub <- msg
		}
	}
}

func (q *queue[E]) Sub(ctx context.Context) (<-chan E, error) {
	return q.sub, nil
}

func (q *queue[E]) Pub(ctx context.Context, evt E) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.pub <- evt:
	}
	return nil
}
