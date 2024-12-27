package actorstore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/ipfs/go-cid"
	"github.com/matryer/is"
	_ "github.com/mattn/go-sqlite3"
)

func TestSQLRepo(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	did := syntax.DID("did:plc:kzvsijt4365vidgqv7o6wksi")
	as := ActorStore{Dir: "./testdata/2/actors", ReadOnly: true}
	r, err := as.Repo(did, testKey(t))
	is.NoErr(err)
	defer r.Close()
	root, err := r.GetRoot(ctx)
	is.NoErr(err)
	is.True(root.ByteLen() > 0)
	rootDetails, err := r.GetRootDetailed(ctx)
	is.NoErr(err)
	is.True(rootDetails.CID.ByteLen() > 0)
	is.True(len(rootDetails.Rev) > 0)
	blocks, err := r.GetBlocks(ctx, []cid.Cid{root})
	is.NoErr(err)
	is.True(blocks.Blocks.Size() > 0)
}

func TestCreateRepo(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	did := syntax.DID("did:plc:kzvsijt4365vidgqv7o6wksi")
	key := must(crypto.GeneratePrivateKeyK256())
	as := ActorStore{Dir: filepath.Join(t.TempDir(), "actors")}
	r, err := as.CreateAsRepo(did, key)
	is.NoErr(err)
	defer r.Close()
	err = r.Tx(ctx, nil, func(ctx context.Context, tx *RepoTransactor) error {
		return nil
	})
	is.NoErr(err)
}

func must[T any](v T, e error) T {
	if e != nil {
		panic(e)
	}
	return v
}
