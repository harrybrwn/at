package atp

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/matryer/is"
)

func init() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)
}

func TestHandleResolver(t *testing.T) {
	ctx := t.Context()
	is := is.New(t)
	r, err := NewDefaultHandleResolver()
	is.NoErr(err)
	for i, fn := range []func(context.Context, string) (syntax.DID, error){
		r.ResolveHandle,
		r.dnsResolver.resolveHandle,
	} {
		did, err := fn(ctx, "bsky.app")
		if err != nil {
			t.Errorf("resolver %d failed: %v", i, err)
			continue
		}
		is.Equal(did, syntax.DID("did:plc:z72i7hdynmk6r22z27h6tvur"))
	}
	did, err := r.wellKnownResolver.resolveHandle(ctx, "aoc.bsky.social")
	is.NoErr(err)
	is.Equal(did, syntax.DID("did:plc:p7gxyfr5vii5ntpwo7f6dhe2"))
}
