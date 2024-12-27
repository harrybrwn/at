package ipldutil

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/codec/dagjson"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/ipld/go-ipld-prime/schema"
	schemadmt "github.com/ipld/go-ipld-prime/schema/dmt"
	"github.com/matryer/is"
)

func TestSimpleAssembler(t *testing.T) {
	is := is.New(t)
	person := "Michael"
	var p string
	err := dagcbor.Decode(&simpleValueAssembler{
		v: reflect.ValueOf(&p).Elem(),
	}, bytes.NewBuffer([]byte{103, 77, 105, 99, 104, 97, 101, 108}))
	is.NoErr(err)
	is.Equal(p, person)
}

func TestSimpleAssembler_AssignInt(t *testing.T) {
	type table struct {
		v reflect.Value
		_ struct{}
	}
	for _, tt := range []table{
		{v: reflect.ValueOf(ptr(0)).Elem()},
		{v: reflect.ValueOf(ptr(int(0))).Elem()},
		{v: reflect.ValueOf(ptr(int8(0))).Elem()},
		{v: reflect.ValueOf(ptr(int16(0))).Elem()},
		{v: reflect.ValueOf(ptr(int32(0))).Elem()},
		{v: reflect.ValueOf(ptr(int64(0))).Elem()},
	} {
		a := simpleValueAssembler{v: tt.v}
		err := a.AssignInt(99)
		if err != nil {
			t.Fatal(err)
		}
		if a.v.Int() != int64(99) {
			t.Errorf("expected 99, got %d", a.v.Int())
		}
	}
	for _, tt := range []table{
		{v: reflect.ValueOf(ptr(uint(0))).Elem()},
		{v: reflect.ValueOf(ptr(uint8(0))).Elem()},
		{v: reflect.ValueOf(ptr(uint16(0))).Elem()},
		{v: reflect.ValueOf(ptr(uint32(0))).Elem()},
		{v: reflect.ValueOf(ptr(uint64(0))).Elem()},
	} {
		a := simpleValueAssembler{v: tt.v}
		err := a.AssignInt(99)
		if err != nil {
			t.Fatal(err)
		}
		if a.v.Uint() != uint64(99) {
			t.Errorf("expected 99, got %d", a.v.Int())
		}
	}
}

func TestListAssembler(t *testing.T) {
	is := is.New(t)
	names := []string{"Sarah", "Alex"}
	node := bindnode.Wrap(&names, schemadmt.TypeSystem.TypeByName("List"))
	var buf bytes.Buffer
	is.NoErr(dagcbor.Encode(node.Representation(), &buf))
	var n []string
	nb := simpleValueAssembler{v: reflect.ValueOf(&n).Elem()}
	is.NoErr(dagcbor.Decode(&nb, &buf))
	is.True(len(n) > 0)
	is.Equal(n, names)
}

func TestMapAssembler(t *testing.T) {
	is := is.New(t)
	type Person struct {
		Name    string    `cbor:"name"`
		Age     int64     `cbor:"age"`
		Friends []string  `cbor:"friends"`
		Id      []byte    `cbor:"id"`
		Cids    []cid.Cid `cbor:"cids"`
		Cid     cid.Cid   `cbor:"cid"`
		Hash    [2]byte   `cbor:"hash"`
		P       *string   `cbor:"p"`
		X       *int      `cbor:"x"`
		B       *bool     `cbor:"b"`
		// parent  *Person   `cbor:"parent"`
	}
	var err error
	var b []byte
	person := Person{
		Name:    "Michael",
		Age:     2,
		Friends: []string{"Sarah", "Alex"},
		Id:      []byte{0xff, 0xaa},
		Cids:    []cid.Cid{cid.MustParse("bafyreic6sbzdnufdmhop4yqldadxsvqq47vva7znyweitpk64rzzrf4z54")},
		Cid:     cid.MustParse("bafyreic6sbzdnufdmhop4yqldadxsvqq47vva7znyweitpk64rzzrf4z54"),
		Hash:    [2]byte{0xfa, 0xca},
		P:       ptr("x"),
		X:       ptr(3),
		B:       ptr(true),
		// parent:  &Person{Name: "Jim"},
	}
	var _ schema.Maybe
	var _ cbornode.Node
	var _ cid.Cid

	// schemadmt.TypeSystem.Accumulate(schema.SpawnStruct("Person", []schema.StructField{
	// 	schema.SpawnStructField("Name", "String", false, false),
	// 	schema.SpawnStructField("Age", "Int", false, false),
	// 	schema.SpawnStructField("Friends", "List", false, false),
	// }, schema.SpawnStructRepresentationMap(map[string]string{})))
	// sch := schemadmt.TypeSystem.TypeByName("Person")

	sch := InferSchema(reflect.TypeOf(&person), &schemadmt.TypeSystem)
	_ = sch

	cbornode.RegisterCborType(Person{})
	b, err = cbornode.DumpObject(&person)
	is.NoErr(err)
	// fmt.Println(b)
	// fmt.Println(cbornode.HumanReadable(b))

	// var buf bytes.Buffer
	// // node := bindnode.Wrap(&person, schemadmt.TypeSystem.TypeByName("Person"))
	// node := bindnode.Wrap(&person, sch)
	// is.NoErr(dagcbor.Encode(node.Representation(), &buf))
	// b = buf.Bytes()
	// fmt.Println(b)
	// fmt.Println(cbornode.HumanReadable(b))

	var p Person
	nb := NodeAssembler(&p)
	err = dagcbor.Decode(nb, bytes.NewBuffer(b))
	if err != nil {
		fmt.Printf("%+v\n", err)
	}
	is.NoErr(err)
	is.Equal(p, person)
}

func TestNode(t *testing.T) {
	is := is.New(t)
	type Person struct {
		Name    string    `cbor:"name"`
		Age     int64     `cbor:"age"`
		Friends []string  `cbor:"friends"`
		Id      []byte    `cbor:"id"`
		Cids    []cid.Cid `cbor:"cids"`
		Cid     *cid.Cid  `cbor:"cid"`
		NilCid  *cid.Cid  `cbor:"nilCid"`
		// Hash    [2]byte   `cbor:"hash"`
		// P       *string   `cbor:"p"`
		X *int  `cbor:"x"`
		B *bool `cbor:"b"`
		C *bool `cbor:"c"`
		// SyntaxCID syntax.CID
	}
	person := Person{
		Name:    "Michael",
		Age:     2,
		Friends: []string{"Sarah", "Alex"},
		// Id:      []byte{0xff, 0xaa},
		Cids: []cid.Cid{cid.MustParse("bafyreic6sbzdnufdmhop4yqldadxsvqq47vva7znyweitpk64rzzrf4z54")},
		Cid:  ptr(cid.MustParse("bafyreic6sbzdnufdmhop4yqldadxsvqq47vva7znyweitpk64rzzrf4z54")),
		// Hash:    [2]byte{0xfa, 0xca},
		// P:       ptr("x"),
		X: ptr(3),
		B: ptr(true),
		C: nil,
		// SyntaxCID: syntax.CID("bafyreic6sbzdnufdmhop4yqldadxsvqq47vva7znyweitpk64rzzrf4z54"),
	}
	var buf bytes.Buffer
	is.NoErr(dagcbor.Encode(BuildNode(&person), &buf))

	cbornode.RegisterCborType(Person{})
	b, err := cbornode.DumpObject(&person)
	is.NoErr(err)
	// fmt.Println(b)
	// fmt.Println(cbornode.HumanReadable(b))
	// fmt.Println(buf.Bytes())
	// fmt.Println(cbornode.HumanReadable(buf.Bytes()))
	is.Equal(b, buf.Bytes())
	var p Person
	is.NoErr(dagcbor.Decode(NodeAssembler(&p), &buf))
	is.Equal(person, p)
}

func TestNodeOmitempty(t *testing.T) {
	is := is.New(t)
	type User struct {
		ID        []byte  `cbor:"id,omitempty"`
		Name      string  `cbor:"name"`
		Key       []byte  `cbor:"key"`
		Age       float64 `cbor:"age"`
		SyntaxCID syntax.CID
	}
	var buf bytes.Buffer
	user := User{
		Name:      "Garry",
		Key:       []byte{1},
		Age:       32.5,
		SyntaxCID: syntax.CID("bafyreic6sbzdnufdmhop4yqldadxsvqq47vva7znyweitpk64rzzrf4z54"),
	}
	is.NoErr(dagcbor.Encode(BuildNode(&user), &buf))
	m := make(map[string]any)
	is.NoErr(cbornode.DecodeInto(buf.Bytes(), &m))
	_, ok := m["id"]
	is.True(!ok)
	is.Equal(m["name"], "Garry")
	is.Equal(m["key"], []byte{1})
	is.Equal(m["age"], 32.5)
	is.Equal(m["SyntaxCID"], cid.MustParse("bafyreic6sbzdnufdmhop4yqldadxsvqq47vva7znyweitpk64rzzrf4z54"))
	var u User
	is.NoErr(dagcbor.Decode(NodeAssembler(&u), &buf))
	is.Equal(user, u)
}

func TestNode_NilCidAndAny(t *testing.T) {
	is := is.New(t)
	type User struct {
		Name string   `cbor:"name"`
		CID  *cid.Cid `cbor:"cid"`
		Data any      `cbor:"data"`
	}
	user := User{
		Name: "Jim",
		CID:  nil,
		Data: map[string]any{
			"$type": "com.atproto.test.data",
			"id":    1,
		},
	}
	var buf bytes.Buffer
	n := BuildNode(&user)
	m, err := n.LookupByString("data")
	is.NoErr(err)
	is.Equal(reflect.Map, m.(*node).v.Kind())
	err = dagcbor.Encode(n, &buf)
	is.NoErr(err)
}

func TestUndefCID(t *testing.T) {
	// t.Skip()
	type Commit struct {
		Blobs  []cid.Cid  `json:"blobs" cborgen:"blobs" cbor:"blobs"`
		Commit cid.Cid    `json:"commit" cborgen:"commit" cbor:"commit"`
		Prev   cid.Cid    `json:"prev,omitempty" cborgen:"prev,omitempty" cbor:"prev,omitempty"`
		Rebase bool       `json:"rebase" cborgen:"rebase" cbor:"rebase"`
		Repo   syntax.DID `json:"repo" cborgen:"repo" cbor:"repo"`
		Rev    string     `json:"rev" cborgen:"rev" cbor:"rev"`
		Seq    int64      `json:"seq" cborgen:"seq" cbor:"seq"`
		Since  string     `json:"since" cborgen:"since" cbor:"since"`
		Time   string     `json:"time" cborgen:"time" cbor:"time"`
		TooBig bool       `json:"tooBig" cborgen:"tooBig" cbor:"tooBig"`
	}
	type User struct {
		// Name string  `cbor:"name"`
		// CID    cid.Cid `cbor:"cid,omitempty"`
		Commit *Commit
	}
	is := is.New(t)
	user := User{
		// Name: "Jim",
		// CID:    cid.Undef,
		Commit: &Commit{
			Commit: cid.MustParse("bafyreic6sbzdnufdmhop4yqldadxsvqq47vva7znyweitpk64rzzrf4z54"),
			Prev:   cid.Undef,
			// Prev: cid.MustParse("bafyreic6sbzdnufdmhop4yqldadxsvqq47vva7znyweitpk64rzzrf4z54"),
		},
	}
	var buf bytes.Buffer
	err := dagcbor.Encode(BuildNode(&user), &buf)
	// fmt.Printf("%+v\n", err)
	is.NoErr(err)
}

var _ dagcbor.DecodeOptions
var _ dagjson.DecodeOptions
