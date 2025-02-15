package ipldutil

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/matryer/is"
)

// go test -run='^$' -bench='.*'

func BenchmarkNodeAssembler(b *testing.B) {
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
		Parent  *Person
	}
	c := cidlink.Link{Cid: cid.MustParse("bafyreic6sbzdnufdmhop4yqldadxsvqq47vva7znyweitpk64rzzrf4z54")}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var p Person
		na := NodeAssembler(&p)
		_ = na
		ma, err := na.BeginMap(0)
		if err != nil {
			b.Fatal(err)
		}
		e, err := ma.AssembleEntry("name")
		if err != nil {
			b.Fatal(err)
		}
		err = e.AssignString("Michael")
		if err != nil {
			b.Fatal(err)
		}
		e, err = ma.AssembleEntry("age")
		if err != nil {
			b.Fatal(err)
		}
		err = e.AssignInt(2)
		if err != nil {
			b.Fatal(err)
		}
		e, err = ma.AssembleEntry("cids")
		if err != nil {
			b.Fatal(err)
		}
		la, err := e.BeginList(1)
		if err != nil {
			b.Fatal(err)
		}
		err = la.AssembleValue().AssignLink(&c)
		if err != nil {
			b.Fatal(err)
		}
		err = la.Finish()
		if err != nil {
			b.Fatal(err)
		}
		pa, err := ma.AssembleEntry("Parent")
		if err != nil {
			b.Fatal(err)
		}
		pma, err := pa.BeginMap(0)
		if err != nil {
			b.Fatal(err)
		}
		e, err = pma.AssembleEntry("name")
		if err != nil {
			b.Fatal(err)
		}
		err = e.AssignString("Jim")
		if err != nil {
			b.Fatal(err)
		}
		err = pma.Finish()
		if err != nil {
			b.Fatal(err)
		}
		err = ma.Finish()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestAssignBytes(t *testing.T) {
	is := is.New(t)

	// slice pointer
	a := simpleValueAssembler{v: reflect.New(goTypeBytes)}
	err := a.AssignBytes([]byte{'a', 'b', 'c'})
	is.NoErr(err)
	b0 := a.v.Interface().(*[]byte)
	is.Equal(*b0, []byte{'a', 'b', 'c'})

	// slice
	a = simpleValueAssembler{v: reflect.New(goTypeBytes).Elem()}
	err = a.AssignBytes([]byte{'a', 'b', 'c'})
	is.NoErr(err)
	b1 := a.v.Interface().([]byte)
	is.Equal(b1, []byte{'a', 'b', 'c'})

	// array with perfect input
	a = simpleValueAssembler{v: reflect.New(reflect.TypeOf([3]byte{})).Elem()}
	err = a.AssignBytes([]byte{'a', 'b', 'c'})
	is.NoErr(err)
	b3 := a.v.Interface().([3]byte)
	is.Equal(b3[:], []byte{'a', 'b', 'c'})

	// array with short input
	a = simpleValueAssembler{v: reflect.New(reflect.TypeOf([4]byte{})).Elem()}
	err = a.AssignBytes([]byte{'a', 'b', 'c'})
	is.NoErr(err)
	b4 := a.v.Interface().([4]byte)
	is.Equal(b4[:], []byte{'a', 'b', 'c', 0x0})

	// input too large
	a = simpleValueAssembler{v: reflect.New(reflect.TypeOf([3]byte{})).Elem()}
	err = a.AssignBytes([]byte{'a', 'b', 'c', 'd'})
	is.True(err != nil)

	// underlying value is wrong array type
	a = simpleValueAssembler{v: reflect.New(reflect.TypeOf([3]uint16{})).Elem()}
	err = a.AssignBytes([]byte{'a', 'b', 'c', 'd'})
	is.True(err != nil)

	// underlying value is wrong slice type
	a = simpleValueAssembler{v: reflect.New(reflect.TypeOf([]uint16{})).Elem()}
	err = a.AssignBytes([]byte{'a', 'b', 'c', 'd'})
	is.True(err != nil)

	// underlying value is wrong type
	a = simpleValueAssembler{v: reflect.New(goTypeBool).Elem()}
	err = a.AssignBytes([]byte{'a', 'b', 'c', 'd'})
	is.True(err != nil && errors.Is(err, ErrWrongType))
}

func TestBeginMap(t *testing.T) {
	is := is.New(t)
	// wrong type
	a := simpleValueAssembler{v: reflect.New(goTypeBool).Elem()}
	_, err := a.BeginMap(0)
	is.True(err != nil)
}

func TestAssignBool(t *testing.T) {
	is := is.New(t)
	// wrong type
	a := simpleValueAssembler{v: reflect.New(goTypeTime).Elem()}
	err := a.AssignBool(true)
	is.True(err != nil && errors.Is(err, ErrWrongType))

	// base case
	a = simpleValueAssembler{v: reflect.New(goTypeBool).Elem()}
	err = a.AssignBool(true)
	is.NoErr(err)
	is.True(a.v.Bool())
}

func TestAssignFloat(t *testing.T) {
	is := is.New(t)

	// wrong type
	a := simpleValueAssembler{v: reflect.New(goTypeBytes).Elem()}
	err := a.AssignFloat(0.0)
	is.True(err != nil && errors.Is(err, ErrWrongType))

	// 32
	a = simpleValueAssembler{v: reflect.New(reflect.TypeOf(float32(0))).Elem()}
	err = a.AssignFloat(1.2)
	is.NoErr(err)
	is.Equal(1.2, math.Round(a.v.Float()*1000)/1000)

	// 64
	a = simpleValueAssembler{v: reflect.New(reflect.TypeOf(float64(0))).Elem()}
	err = a.AssignFloat(1.2)
	is.NoErr(err)
	is.Equal(1.2, math.Round(a.v.Float()*1000)/1000)
}

func TestAssignInt(t *testing.T) {
	is := is.New(t)
	// wrong type
	a := simpleValueAssembler{v: reflect.New(goTypeBytes).Elem()}
	err := a.AssignInt(33)
	is.True(err != nil && errors.Is(err, ErrWrongType))
}

func TestAssignString(t *testing.T) {
	is := is.New(t)
	// wrong type
	a := simpleValueAssembler{v: reflect.New(goTypeBytes).Elem()}
	err := a.AssignString("hello?")
	is.True(err != nil && errors.Is(err, ErrWrongType))
}

func TestAssignLink(t *testing.T) {
	c := must(cid.Decode("bafkreidpjxht62vut32z45pyonzch3cwqzws4lw6qklgbid5nyh4j3qtbi"))
	cl := cidlink.Link{Cid: c}
	is := is.New(t)
	// wrong type
	a := simpleValueAssembler{v: reflect.New(goTypeInt).Elem()}
	err := a.AssignLink(nil)
	is.True(err != nil && errors.Is(err, ErrWrongType))

	a = simpleValueAssembler{v: reflect.New(goTypeCid).Elem()}
	is.NoErr(a.AssignLink(&cl))
	a = simpleValueAssembler{v: reflect.New(goTypeCidLink).Elem()}
	is.NoErr(a.AssignLink(&cl))
	var v any
	a = simpleValueAssembler{v: reflect.ValueOf(&v)}
	is.NoErr(a.AssignLink(&cl))
	is.Equal(v, (any)(map[string]any{"$link": c}))

	cl = cidlink.Link{Cid: cid.Cid{}}
	a = simpleValueAssembler{v: reflect.New(goTypeCid).Elem()}
	err = a.AssignLink(&cl)
	is.True(err != nil && errors.Is(err, cid.ErrCidTooShort))
	a = simpleValueAssembler{v: reflect.New(goTypeCidLink).Elem()}
	err = a.AssignLink(&cl)
	is.True(err != nil && errors.Is(err, cid.ErrCidTooShort))
	a = simpleValueAssembler{v: reflect.New(goTypeSyntaxCID).Elem()}
	err = a.AssignLink(&cl)
	is.True(err != nil)
	a = simpleValueAssembler{v: reflect.ValueOf(&v)}
	err = a.AssignLink(&cl)
	is.True(err != nil && errors.Is(err, cid.ErrCidTooShort))
}

func TestAssembleNode(t *testing.T) {
	t.Skip()
	type X struct {
		A *int // null
		B bool
		C int32
		D float32
		E string
		F []byte
		G cid.Cid
		H map[string]any
		I []any
	}
	is := is.New(t)
	n := BuildNode(&X{
		A: nil, B: true, C: 22, D: 9.8, E: "hello?", F: []byte("le criox"),
		G: must(cid.Decode("bafkreidpjxht62vut32z45pyonzch3cwqzws4lw6qklgbid5nyh4j3qtbi")),
		H: map[string]any{"a": 1, "b": true, "d": 6.6, "e": "e"},
		// I: []any{1, "two", '3', []byte{'4'}, nil},
	})
	a := simpleValueAssembler{v: reflect.New(goTypeInt).Elem()}
	err := a.AssignNode(n)
	is.NoErr(err)
}
