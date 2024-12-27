package pubsub

import (
	"context"
	"io"
	"iter"
)

// Pub is a publisher. The K type parameter is the type of the routing key. The
// T type parameter is the event type.
type Pub[T any] interface {
	io.Closer
	Pub(ctx context.Context, v T) error
}

// Sub is a subscriber. The K type parameter is the type of the routing key. The
// T type parameter is the event type.
type Sub[T any] interface {
	io.Closer
	Sub(ctx context.Context) (<-chan T, error)
}

// Bus is a type that creates publishers and subscribers.
type Bus[Opt, E any] interface {
	io.Closer
	Publisher(ctx context.Context, opts ...Opt) (Pub[E], error)
	Subscriber(ctx context.Context, opts ...Opt) (Sub[E], error)
}

func Subscribe[O, E any](ctx context.Context, bus Bus[O, E]) (iter.Seq[E], error) {
	sub, err := bus.Subscriber(ctx)
	if err != nil {
		return nil, err
	}
	events, err := sub.Sub(ctx)
	if err != nil {
		return nil, err
	}
	return func(yield func(E) bool) {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-events:
				if !ok || !yield(e) {
					return
				}
			}
		}
	}, nil
}

func Publish[O, E any](ctx context.Context, opt O, evt E, bus Bus[O, E]) error {
	pub, err := bus.Publisher(ctx, opt)
	if err != nil {
		return err
	}
	defer pub.Close()
	return pub.Pub(ctx, evt)
}
