package actorstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/harrybrwn/at/api/app/bsky"
	"github.com/harrybrwn/at/internal/repo"
	"github.com/harrybrwn/at/internal/xiter"
	"github.com/ipfs/go-cid"
	"github.com/matryer/is"
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	l := slog.New(slog.NewTextHandler(
		os.Stdout,
		&slog.HandlerOptions{Level: slog.LevelInfo},
	))
	slog.SetDefault(l)
}

func TestSQLRepo(t *testing.T) {
	is := is.New(t)
	ctx := t.Context()
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
	ctx := t.Context()
	did := "did:plc:nsu4iq7726acidyqpha2zuk3"
	key := must(crypto.GeneratePrivateKeyK256())
	as, blob := teststore(t, did, key)
	_ = blob
	rr, err := as.Repo(syntax.DID(did), key)
	is.NoErr(err)
	err = as.Transact(ctx, syntax.DID(did), blob, func(ctx context.Context, tx *ActorStoreTransactor) error {
		_, err := tx.Repo.CreateRepo(ctx, nil)
		return err
	})
	is.NoErr(err)
	root, err := rr.GetRootDetailed(ctx)
	is.NoErr(err)
	allBlocks, err := rr.ListAllBlocks(ctx)
	is.NoErr(err)
	is.True(xiter.Contains(xiter.Keys(allBlocks.Iter()), root.CID))
}

func TestFormatCommit(t *testing.T) {
	is := is.New(t)
	ctx := t.Context()
	did := "did:plc:nsu4iq7726acidyqpha2zuk3"
	key, err := crypto.GeneratePrivateKeyK256()
	is.NoErr(err)
	as, blob := teststore(t, did, key)
	_ = blob
	err = as.Transact(ctx, syntax.DID(did), blob, func(ctx context.Context, tx *ActorStoreTransactor) error {
		_, err := tx.Repo.CreateRepo(ctx, nil)
		return err
	})
	is.NoErr(err)
	rr, err := as.Repo(syntax.DID(did), key)
	is.NoErr(err)
	root, err := rr.GetRoot(ctx)
	is.NoErr(err)
	allKnownBlocks, err := rr.ListAllBlocks(ctx)
	is.NoErr(err)
	_ = allKnownBlocks
	writes := []repo.PreparedWrite{
		repo.PrepWrite(&repo.PreparedCreate{
			URI: syntax.ATURI(fmt.Sprintf("at://%s/app.bsky.actor.profile/self", did)),
			Record: &bsky.ActorProfile{
				LexiconTypeID: "app.bsky.actor.profile",
				DisplayName:   "Christopher",
			},
		}),
		repo.PrepWrite(&repo.PreparedCreate{
			URI: syntax.ATURI(fmt.Sprintf("at://%s/app.bsky.feed.post/0", did)),
			Record: &bsky.FeedPost{
				LexiconTypeID: "app.bsky.feed.post",
				CreatedAt:     time.Now().Format(time.RFC1123Z),
				Langs:         []syntax.Language{"en"},
				Text:          "this is a test post",
			},
		}),
	}
	err = as.Transact(ctx, syntax.DID(did), blob, func(ctx context.Context, tx *ActorStoreTransactor) error {
		commit, err := tx.Repo.FormatCommit(ctx, writes, cid.Undef)
		if err != nil {
			return err
		}
		is.True(!xiter.Contains(xiter.Keys(allKnownBlocks.Iter()), commit.CID))
		_, err = tx.Record.GetByRev(ctx, commit.Rev)
		is.True(errors.Is(err, sql.ErrNoRows)) // make sure the commit hasn't been stored
		for c := range xiter.Keys(commit.NewBlocks.Iter()) {
			if xiter.Contains(xiter.Keys(allKnownBlocks.Iter()), c) {
				t.Errorf("%s should not be in the list of known blocks", c)
			}
		}
		is.True(commit.RemovedCIDs.Has(root))            // should mark old root as removed
		is.Equal(commit.NewBlocks.Size(), len(writes)+1) // number of writes plus the commit
		is.True(commit.NewBlocks.Has(commit.CID))
		is.Equal(commit.RelevantBlocks.Size(), 1)
		is.True(commit.RelevantBlocks.Has(commit.CID))
		return nil
	})
	if err != nil {
		fmt.Printf("%+v\n", err)
	}
	is.NoErr(err)
}

func teststore(t *testing.T, did string, key *crypto.PrivateKeyK256) (*ActorStore, *repo.DiskBlobStore) {
	t.Helper()
	as := ActorStore{Dir: filepath.Join(t.TempDir(), "actors")}
	blobtmp := filepath.Join(t.TempDir(), "blobs", "temp")
	blobquarentine := filepath.Join(t.TempDir(), "blobs", "quarentine")
	blob := repo.NewDiskBlobStore(did,
		filepath.Join(t.TempDir(), "blobs"),
		blobtmp,
		blobquarentine,
	)
	dir, _, keyfile := as.location(syntax.DID(did))
	for _, d := range []string{
		dir,
		blobtmp,
		blobquarentine,
	} {
		err := os.MkdirAll(d, 0755)
		if err != nil {
			t.Fatalf("failed to create actorstore directory %q: %v", d, err)
		}
	}
	err := os.WriteFile(keyfile, key.Bytes(), 0644)
	if err != nil {
		t.Fatalf("failed to write private key to %q", keyfile)
	}
	_, err = as.migrate(syntax.DID(did), key)
	if err != nil {
		t.Fatal(err)
	}
	return &as, blob
}

func must[T any](v T, e error) T {
	if e != nil {
		panic(e)
	}
	return v
}
