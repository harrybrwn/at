package repo

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"reflect"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/repo"
	"github.com/fxamacker/cbor/v2"
	"github.com/harrybrwn/db"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/matryer/is"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/api/app/bsky"
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
	is.Equal(commit.Prev, cid.Undef)
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
			Collection: "app.bsky.actor.profile",
			RecordKey:  "self",
			Record: &bsky.ActorProfile{
				LexiconTypeID: "app.bsky.actor.profile",
				DisplayName:   "Jimmy",
				CreatedAt:     time.Now().Format(time.RFC3339),
			},
		},
		{
			Action:     WriteOpActionUpdate,
			Collection: "fries.in.the",
			RecordKey:  "bag",
			Record:     "",
		},
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
	if err != nil {
		fmt.Printf("%+v\n", err)
	}
	is.NoErr(err)
	is.True(commit.CID.ByteLen() > 0)
}

func TestLoadRepo(t *testing.T) {
	ctx := context.Background()
	did := "did:plc:nsu4iq7726acidyqpha2zuk3"
	is := is.New(t)
	ds, err := sqlite.File(
		fmt.Sprintf("testdata/3/actors/b6/%s/store.sqlite", did),
		sqlite.ReadOnly,
	)
	is.NoErr(err)
	defer ds.Close()
	root, rev, err := rootCid(ds, did)
	is.NoErr(err)
	bs := blockstore.NewSQLStore(db.New(ds), rev)
	repo, err := Load(ctx, bs, root, signer(nil))
	is.NoErr(err)
	is.Equal(repo.commit.Rev, "3lhmfbobbfk2k")
}

func TestCommit(t *testing.T) {
	ctx := context.Background()
	did := "did:plc:nsu4iq7726acidyqpha2zuk3"
	is := is.New(t)
	bs := blockstore.InMemory()
	r := New(did, bs, hardsigner)
	is.True(r.dirty)
	is.Equal(r.commit.DID, did)
	is.Equal(r.commit.Sig, nil)
	c, rev, err := r.Commit(ctx)
	is.NoErr(err)
	is.True(len(rev) > 0)
	is.True(!c.Equals(cid.Undef))
	is.True(!r.dirty)
	is.Equal(r.commit.DID, did)
	is.True(len(r.commit.Sig) > 0)
	_, err = bs.Get(ctx, c)
	is.NoErr(err)
}

func TestRepoPutRecord(t *testing.T) {
	ctx := context.Background()
	did := "did:plc:nsu4iq7726acidyqpha2zuk3"
	is := is.New(t)
	database, err := sqlite.InMemory(sqlite.JournalMode("WAL"))
	is.NoErr(err)
	defer database.Close()
	bs := blockstore.NewSQLStore(db.New(database), "")
	is.NoErr(bs.Migrate(ctx))
	// bs := blockstore.InMemory()
	key, err := getKey()
	is.NoErr(err)
	r := New(did, bs, signer(key))
	var c cid.Cid
	_, err = r.PutRecord(ctx, "app.bsky.actor.profile", "self", &bsky.ActorProfile{
		LexiconTypeID: "app.bsky.actor.profile",
		DisplayName:   "Jimmy",
		CreatedAt:     time.Now().Format(time.RFC3339),
	})
	is.NoErr(err)
	c, _, err = r.Commit(ctx)
	is.NoErr(err)
	fmt.Println(c)
	// _, err = r.PutRecord(ctx, "app.test", "x", "test-record")
	// is.NoErr(err)
	// c, _, err = r.Commit(ctx)
	// is.NoErr(err)
	// _ = c
	// c, err = r.mst.GetPointer(ctx)
	// is.NoErr(err)
	// fmt.Println(c)
	r, err = Load(ctx, bs, c, signer(key))
	is.NoErr(err)
	c, err = r.mst.GetPointer(ctx)
	is.NoErr(err)
	fmt.Println(c)
	var p bsky.ActorProfile
	is.NoErr(r.GetRecord(ctx, "app.bsky.actor.profile", "self", &p))
	is.Equal(p.DisplayName, "Jimmy")
}

func TestRepoGetRecord(t *testing.T) {
	t.Skip()
	is := is.New(t)
	ctx := context.Background()
	did := "did:plc:kzvsijt4365vidgqv7o6wksi"
	database, err := sqlite.File(
		"testdata/2/actors/02/did:plc:kzvsijt4365vidgqv7o6wksi/store.sqlite",
		sqlite.ReadOnly,
	)
	is.NoErr(err)
	defer database.Close()
	key := readkey("testdata/2/actors/02/did:plc:kzvsijt4365vidgqv7o6wksi/key")
	root, rev, err := rootCid(database, did)
	is.NoErr(err)
	r, err := Load(ctx, blockstore.NewSQLStore(db.New(database), rev), root, signer(key))
	is.NoErr(err)
	fmt.Println("GerRecordBytes")
	_, rawRecord, err := r.GetRecordBytes(ctx, "app.bsky.actor.profile", "self")
	if err != nil {
		fmt.Printf("%+v\n", err)
	}
	is.NoErr(err)
	var p bsky.ActorProfile
	is.NoErr(dagcbor.Decode(ipldutil.NodeAssembler(&p), bytes.NewBuffer(rawRecord)))
	fmt.Printf("%+v\n", p)
}

func TestNewRepo(t *testing.T) {
	t.Skip()
	ctx := context.Background()
	did := "did:plc:nsu4iq7726acidyqpha2zuk3"
	is := is.New(t)
	db, err := sqlite.File(
		fmt.Sprintf("testdata/3/actors/b6/%s/store.sqlite", did),
		sqlite.ReadOnly,
	)
	key := readkey(fmt.Sprintf("testdata/3/actors/b6/%s/key", did))
	is.NoErr(err)
	defer db.Close()
	// root, err := rootCid(db, did)
	// is.NoErr(err)
	// fmt.Printf("expected root cid: %+v\n", root)

	cid, _, err := repo.NewRepo(ctx, did, blockstore.InMemory()).Commit(ctx, func(ctx context.Context, did string, b []byte) ([]byte, error) {
		fmt.Printf("%v\n", b)
		return key.HashAndSign(b)
	})
	is.NoErr(err)
	fmt.Println("         root cid:", cid.String())
	cid, _, err = repo.NewRepo(ctx, did, blockstore.InMemory()).Commit(ctx, func(ctx context.Context, did string, b []byte) ([]byte, error) {
		return key.HashAndSign(b)
	})
	is.NoErr(err)
	fmt.Println("         root cid:", cid.String())
	// return
	// commit, err := FormatInitCommit(
	// 	ctx, blockstore.InMemory(), did,
	// 	readkey(fmt.Sprintf("testdata/3/actors/b6/%s/key", did)),
	// 	nil,
	// )
	// is.NoErr(err)
	// fmt.Println("         root cid:", commit.CID.String())
	// // fmt.Printf("%#v\n", commit)
	// commit, err = FormatInitCommit(ctx, blockstore.InMemory(), did, key, nil)
	// is.NoErr(err)
	// fmt.Println("         root cid:", commit.CID.String())
	// commit, err = FormatInitCommit(ctx, blockstore.InMemory(), did, key, nil)
	// is.NoErr(err)
	// fmt.Println("         root cid:", commit.CID.String())
}

func rootCid(d *sql.DB, did string) (cid.Cid, string, error) {
	var rawCid, rev string
	err := d.QueryRow("SELECT cid, rev FROM repo_root WHERE did = ?", did).Scan(&rawCid, &rev)
	if err != nil {
		return cid.Undef, "", err
	}
	c, err := cid.Parse(rawCid)
	if err != nil {
		return cid.Undef, "", err
	}
	return c, rev, nil
}

func ok[T any](v T, ok bool) T {
	if !ok {
		panic("not ok")
	}
	return v
}

func readkey(path string) *crypto.PrivateKeyK256 {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	k, err := crypto.ParsePrivateBytesK256(b)
	if err != nil {
		panic(err)
	}
	return k
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

func getKey() (*crypto.PrivateKeyK256, error) {
	rawkey := []byte{
		0x54, 0x54, 0x39, 0xeb, 0xd0, 0xf3, 0x5e, 0x29, 0xcc, 0x5e, 0xf1,
		0xf3, 0x5e, 0xa5, 0x8, 0x95, 0x6b, 0xeb, 0xb1, 0x25, 0x91, 0x2d,
		0x35, 0xf0, 0xa3, 0x30, 0x62, 0x8e, 0x3e, 0xe8, 0x92, 0xeb}
	key, err := crypto.ParsePrivateBytesK256(rawkey)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func hardsigner(ctx context.Context, did string, data []byte) ([]byte, error) {
	if did != "did:plc:nsu4iq7726acidyqpha2zuk3" {
		return nil, errors.New("wrong did")
	}
	key, err := getKey()
	if err != nil {
		return nil, err
	}
	return key.HashAndSign(data)
}

func signer(k crypto.PrivateKey) Signer {
	return func(ctx context.Context, did string, data []byte) ([]byte, error) {
		return k.HashAndSign(data)
	}
}
