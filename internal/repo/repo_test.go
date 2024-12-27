package repo

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path"
	"reflect"
	"testing"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/fxamacker/cbor/v2"
	"github.com/harrybrwn/db"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/matryer/is"

	"github.com/harrybrwn/at/internal/blockstore"
	"github.com/harrybrwn/at/internal/ipldutil"
	"github.com/harrybrwn/at/internal/sqlite"
)

func TestRepo(t *testing.T) {
}

func TestFormatInitCommit(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	did := createFakeDID()
	key := must(crypto.GeneratePrivateKeyK256())
	blocks := blockstore.InMemory()
	commit, err := FormatInitCommit(ctx, blocks, did, key, nil)
	is.NoErr(err)
	is.True(commit.CID.ByteLen() > 0)
	is.Equal(commit.Prev, "")
	is.Equal(commit.Since, "")
	is.Equal(commit.RemovedCIDs.Len(), 0)
	is.Equal(commit.NewBlocks.Size(), 1)
	is.Equal(commit.RelevantBlocks.Size(), 1)
	is.Equal(
		ok(commit.RelevantBlocks.Get(commit.CID)),
		ok(commit.NewBlocks.Get(commit.CID)),
	)
	blocks.Clear()
	commit, err = FormatInitCommit(ctx, blocks, did, key, []RecordWriteOp{
		{
			Action:     WriteOpActionCreate,
			Collection: "me.hrry.test.post",
			RecordKey:  "123",
			Record: map[string]any{
				"$type": "me.hrry.test.post",
				"title": "Test Post",
				"body":  "this is a test",
			},
		},
	})
	is.NoErr(err)
	is.True(commit.CID.ByteLen() > 0)
}

func ok[T any](v T, ok bool) T {
	if !ok {
		panic("not ok")
	}
	return v
}

var decMode cbor.DecMode

func init() {
	tags := cbor.NewTagSet()
	err := tags.Add(
		cbor.TagOptions{
			EncTag: cbor.EncTagRequired,
			DecTag: cbor.DecTagOptional,
		},
		reflect.TypeOf(cid.Cid{}),
		42,
	)
	if err != nil {
		panic(err)
	}
	decMode, err = cbor.DecOptions{
		MapKeyByteString:  cbor.MapKeyByteStringAllowed,
		BinaryUnmarshaler: cbor.BinaryUnmarshalerByteString,
		DefaultMapType:    reflect.TypeOf(map[string]any{}),
	}.DecModeWithSharedTags(tags)
	if err != nil {
		panic(err)
	}
}

func TestCBOR(t *testing.T) {
	did := os.Getenv("BSKY_TEST_DID")
	if len(did) == 0 {
		t.Skip()
	}
	cbornode.RegisterCborType(UnsignedCommit{})
	cbornode.RegisterCborType(SignedCommit{})
	is := is.New(t)
	d, err := sqlite.File(
		path.Join("testdata/2/actors/02", did, "store.sqlite"),
		sqlite.ReadOnly,
	)
	is.NoErr(err)
	defer d.Close()
	rows, err := d.Query("SELECT cid, rev FROM repo_root LIMIT 1")
	is.NoErr(err)
	var (
		rawcid, rev string
		content     []byte
	)
	is.NoErr(db.ScanOne(rows, &rawcid, &rev))
	rows, err = d.Query("SELECT content FROM repo_block WHERE cid = ?", rawcid)
	is.NoErr(err)
	is.NoErr(db.ScanOne(rows, &content))

	var s SignedCommit
	is.NoErr(dagcbor.Decode(ipldutil.NodeAssembler(&s), bytes.NewBuffer(content)))
	if len(s.Data.Bytes()) == 0 {
		t.Error("should have a 'data' cid")
	}
}

func createFakeDID() string {
	var buf [8]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		panic(err)
	}
	return "did:plc:" + hex.EncodeToString(buf[:])
}
