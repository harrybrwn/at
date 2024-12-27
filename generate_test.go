package main

import (
	"fmt"
	"testing"

	"github.com/matryer/is"
	cborgen "github.com/whyrusleeping/cbor-gen"

	"github.com/harrybrwn/at/api/com/atproto"
)

func TestCbor(t *testing.T) {
	t.Skip()
	is := is.New(t)
	var _ cborgen.CborBool
	var r atproto.RepoGetRecordResponse
	info, err := cborgen.ParseTypeInfo(&r)
	is.NoErr(err)
	// err = cborgen.PrintHeaderAndUtilityMethods(os.Stdout, "github.com/harrybrwn/at/api", []*cborgen.GenTypeInfo{info})
	// is.NoErr(err)
	// cborgen.GenMapEncodersForType(info, os.Stdout)
	// // cborgen.GenTupleEncodersForType(info, os.Stdout)
	// fmt.Println()

	fmt.Println(info.Name)
	for _, f := range info.Fields {
		// fmt.Println(f.Name, f.Pkg, f.MapKey, f.OmitEmpty, f.IsArray(), f.Type)
		fmt.Printf("%#v\n", f.Type)
	}
}
