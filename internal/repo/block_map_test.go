package repo

import (
	"fmt"
	"testing"

	"github.com/fxamacker/cbor/v2"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/matryer/is"
	"github.com/polydawn/refmt/obj/atlas"
)

func TestBlockMap(t *testing.T) {
	is := is.New(t)
	bm := NewBlockMap()
	bm.Set(must(newCid([]byte("cid"))), []byte("value"))
	res, ok := bm.Get(must(newCid([]byte("cid"))))
	is.True(ok)
	is.Equal(res, []byte("value"))
	bm.Delete(must(newCid([]byte("cid"))))
	_, ok = bm.Get(must(newCid([]byte("cid"))))
	is.True(!ok)
}

func TestCBORMarshalling(t *testing.T) {
	t.Skip()
	is := is.New(t)
	// cbornode.RegisterCborType(RecordWriteOp{})
	cbornode.RegisterCborType(atlas.BuildEntry(RecordWriteOp{}).
		StructMap().
		AutogenerateWithSortingScheme(atlas.KeySortMode_RFC7049).
		Complete())
	// opts := cbor.CanonicalEncOptions()
	opts := cbor.CoreDetEncOptions()
	// opts := cbor.EncOptions{
	// 	// Sort: cbor.SortCanonical,
	// 	Sort: cbor.SortLengthFirst,
	// 	// FieldName: cbor.FieldNameToTextString,
	// }
	em, err := opts.EncMode()
	is.NoErr(err)

	v := RecordWriteOp{
		Action:     WriteOpActionCreate,
		Collection: "app.bsky.feed.like",
		RecordKey:  "9lfn636dmxc2m",
		Record: map[string]any{
			"$type": "app.bsky.feed.like",
			"test":  true,
		},
	}
	b1, err := em.Marshal(&v)
	is.NoErr(err)
	b2, err := cbornode.DumpObject(&v)
	is.NoErr(err)
	fmt.Printf("%s\n", b1)
	fmt.Printf("%s\n", b2)
	fmt.Println(cbornode.HumanReadable(b1))
	fmt.Println(cbornode.HumanReadable(b2))
	var m any
	// cbor.Unmarshal(b1, &m)
	cbornode.DecodeInto(b1, &m)
	fmt.Printf("%#v\n", m)
}

func must[T any](v T, e error) T {
	if e != nil {
		panic(e)
	}
	return v
}
