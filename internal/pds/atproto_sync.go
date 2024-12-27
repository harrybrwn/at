package pds

import (
	"context"
	"iter"

	"github.com/harrybrwn/at/api/com/atproto"
)

func (pds *PDS) SubscribeRepos(ctx context.Context, params *atproto.SyncSubscribeReposParams) (iter.Seq[*atproto.SyncSubscribeReposUnion], error) {
	ctx, cancel := context.WithCancel(ctx)
	sub, err := pds.Bus.Subscriber(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	events, err := sub.Sub(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	return func(yield func(*atproto.SyncSubscribeReposUnion) bool) {
		defer cancel()
		defer sub.Close()
		for evt := range events {
			if !yield((*atproto.SyncSubscribeReposUnion)(evt.Event)) {
				return
			}
		}
	}, nil
}

var (
	_ atproto.SyncSubscribeRepos = (*PDS)(nil)
)
