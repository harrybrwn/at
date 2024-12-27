package pubsub

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestChannelBus(t *testing.T) {
	type Event struct {
		Name string
	}
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*10)
	defer cancel()
	bus := ChannelBus[*Event]{queues: make([]queue[*Event], 0)}
	seq, err := Subscribe(ctx, &bus)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := bus.Publisher(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	go func() {
		err := pub.Pub(ctx, &Event{Name: "Jim"})
		if err != nil {
			panic(err)
		}
		err = pub.Pub(ctx, &Event{Name: "Garry"})
		if err != nil {
			panic(err)
		}
	}()

	results := slices.Collect(seq)
	if results[0].Name != "Jim" {
		t.Error("wrong name")
	}
	if results[1].Name != "Garry" {
		t.Error("wrong name")
	}
	err = bus.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func collect[E any](ctx context.Context, ch <-chan E) []E {
	res := make([]E, 0)
	for {
		select {
		case <-ctx.Done():
			return res
		case elem, ok := <-ch:
			if !ok {
				return res
			}
			res = append(res, elem)
		}
	}
}
