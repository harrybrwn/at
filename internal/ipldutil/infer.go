package ipldutil

import (
	stderrors "errors"
	"fmt"
	"reflect"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/schema"
	schemadmt "github.com/ipld/go-ipld-prime/schema/dmt"
)

var ErrWrongType = stderrors.New("wrong type")

const maxRecursionLevel = 1 << 10

func InferSchema(typ reflect.Type, sys *schema.TypeSystem) schema.Type {
	return inferSchema(typ, sys, 0)
}

var (
	goTypeBool         = reflect.TypeOf(false)
	goTypeInt          = reflect.TypeOf(int(0))
	goTypeFloat        = reflect.TypeOf(0.0)
	goTypeString       = reflect.TypeOf("")
	goTypeBytes        = reflect.TypeOf([]byte{})
	goTypeLink         = reflect.TypeOf((*datamodel.Link)(nil)).Elem()
	goTypeNode         = reflect.TypeOf((*datamodel.Node)(nil)).Elem()
	goTypeCidLink      = reflect.TypeOf(cidlink.Link{})
	goTypeCidLinkPtr   = reflect.TypeOf(&cidlink.Link{})
	goTypeCid          = reflect.TypeOf(cid.Cid{})
	goTypeCidPtr       = reflect.TypeOf(&cid.Cid{})
	goTypeSyntaxCID    = reflect.TypeOf(syntax.CID(""))
	goTypeSyntaxCIDPtr = reflect.TypeOf(ptr(syntax.CID("")))
	goTypeTime         = reflect.TypeOf(time.Time{})
	goTypeAny          = reflect.TypeOf((any)(nil))

	schemaTypeBool   = schema.SpawnBool("Bool")
	schemaTypeInt    = schema.SpawnInt("Int")
	schemaTypeFloat  = schema.SpawnFloat("Float")
	schemaTypeString = schema.SpawnString("String")
	schemaTypeBytes  = schema.SpawnBytes("Bytes")
	schemaTypeLink   = schema.SpawnLink("Link")
	schemaTypeCID    = schema.SpawnLink("CID")
	schemaTypeAny    = schema.SpawnAny("Any")
)

// inferSchema can build a schema from a Go type
func inferSchema(typ reflect.Type, sys *schema.TypeSystem, level int) schema.Type {
	if level > maxRecursionLevel {
		panic(fmt.Sprintf("inferSchema: refusing to recurse past %d levels", maxRecursionLevel))
	}
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Bool:
		return schemaTypeBool
	case reflect.Float32, reflect.Float64:
		return schemaTypeFloat
	case reflect.String:
		return schemaTypeString
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return schemaTypeInt
	case reflect.Struct:
		// these types must match exactly since we need symmetry of being able to
		// get the values an also assign values to them
		if typ == goTypeCidLink {
			return schemaTypeLink
		}
		if typ == goTypeCid {
			return schemaTypeCID
		}
		name := typ.Name()
		if found := sys.TypeByName(name); found != nil {
			return found
		}

		fieldsSchema := make([]schema.StructField, typ.NumField())
		for i := range fieldsSchema {
			field := typ.Field(i)
			fieldName := field.Name
			optional := false
			rawTag, ok := field.Tag.Lookup("cbor")
			opts := new(tagOpt)
			if ok {
				opts, _ = parseTag(rawTag)
			}
			if len(opts.name) > 0 {
				fieldName = opts.name
			}
			optional = opts.omitempty
			ftyp := field.Type
			nullable := ftyp.Kind() == reflect.Pointer
			ftypSchema := inferSchema(ftyp, sys, level+1)
			fieldsSchema[i] = schema.SpawnStructField(
				fieldName, // TODO: allow configuring the name with tags
				ftypSchema.Name(),
				// TODO: support nullable/optional with tags
				optional && nullable,
				nullable,
			)
		}
		if name == "" {
			panic("TODO: anonymous composite types")
		}
		typSchema := schema.SpawnStruct(name, fieldsSchema, nil)
		schemadmt.TypeSystem.Accumulate(typSchema)
		return typSchema
	case reflect.Array:
		if typ.Elem().Kind() == reflect.Uint8 {
			// Special case for []byte.
			return schemaTypeBytes
		}
	case reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			// Special case for []byte.
			return schemaTypeBytes
		}

		nullable := false
		if typ.Elem().Kind() == reflect.Ptr {
			nullable = true
		}
		etypSchema := inferSchema(typ.Elem(), sys, level+1)
		name := typ.Name()
		if name == "" {
			name = "List_" + etypSchema.Name()
		}
		if sch := sys.TypeByName(name); sch != nil {
			return sch
		}
		typSchema := schema.SpawnList(name, etypSchema.Name(), nullable)
		// schemadmt.TypeSystem.Accumulate(typSchema)
		sys.Accumulate(typSchema)
		return typSchema
	case reflect.Interface:
		// these types must match exactly since we need symmetry of being able to
		// get the values an also assign values to them
		if typ == goTypeLink {
			return schemaTypeLink
		}
		if typ == goTypeNode {
			return schemaTypeAny
		}
		if typ.Implements(goTypeLink) {
			return schemaTypeLink
		}
		if typ.Implements(goTypeNode) {
			return schemaTypeAny
		}
		panic("unable to infer from interface")
	}
	panic(fmt.Sprintf("unable to infer from type %s", typ.Kind().String()))
}
