package ipldutil

import (
	"reflect"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/pkg/errors"
)

func BuildNode(value any) datamodel.Node {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	n := node{v: v}
	if v.Kind() == reflect.Struct {
		typ := v.Type()
		return &structNode{
			node:  n,
			typ:   typ,
			pairs: buildPairs(v, typ),
		}
	}
	return &n
}

type node struct {
	v reflect.Value
}

var _ datamodel.Node = (*node)(nil)

func (n *node) Kind() datamodel.Kind {
	switch n.v.Kind() {
	case reflect.Bool:
		return datamodel.Kind_Bool
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64:
		return datamodel.Kind_Int
	case reflect.Float32, reflect.Float64:
		return datamodel.Kind_Float
	case reflect.String:
		if n.v.Type() == goTypeSyntaxCID {
			return datamodel.Kind_Link
		}
		return datamodel.Kind_String
	case reflect.Array, reflect.Slice:
		switch n.v.Type().Elem().Kind() {
		case reflect.Uint8:
			if n.v.Len() == 0 {
				return datamodel.Kind_Null
			}
			return datamodel.Kind_Bytes
		}
		return datamodel.Kind_List
	case reflect.Map, reflect.Struct:
		switch n.v.Type() {
		case goTypeCid,
			goTypeCidLink,
			goTypeSyntaxCID:
			return datamodel.Kind_Link
		case
			goTypeCidPtr,
			goTypeCidLinkPtr,
			goTypeSyntaxCIDPtr:
			return datamodel.Kind_Link
		}
		return datamodel.Kind_Map
	case reflect.Interface:
		return datamodel.Kind_Invalid
	default:
		return datamodel.Kind_Invalid
	}
}

func (n *node) LookupByString(key string) (datamodel.Node, error) {
	panic("LookupByString not implemented")
}

func (n *node) LookupByNode(key datamodel.Node) (datamodel.Node, error) {
	panic("LookupByNode not implemented")
}

func (n *node) LookupByIndex(idx int64) (datamodel.Node, error) {
	if n.v.Kind() == reflect.Struct {
		return &node{v: n.v.Field(int(idx))}, nil
	}
	return &node{v: n.v.Index(int(idx))}, nil
}

func (n *node) LookupBySegment(seg datamodel.PathSegment) (datamodel.Node, error) {
	panic("LookupBySegment not implemented")
}

func (n *node) MapIterator() datamodel.MapIterator {
	if n.v.Kind() == reflect.Struct {
		typ := n.v.Type()
		pairs := buildPairs(n.v, typ)
		it := structMapIterator{
			node: &structNode{
				node:  node{v: n.v},
				typ:   typ,
				pairs: pairs,
			},
		}
		return &it
	}
	return &mapIterator{
		iter: n.v.MapRange(),
		len:  n.v.Len(),
	}
}

func (n *node) ListIterator() datamodel.ListIterator {
	panic("ListIterator not implemented")
}

func (n *node) Length() int64 {
	if n.v.Kind() == reflect.Struct {
		return int64(n.v.NumField())
	}
	return int64(n.v.Len())
}

func (n *node) IsAbsent() bool { return n.v.IsZero() }
func (n *node) IsNull() bool   { return n.v.IsNil() }

func (n *node) AsBool() (bool, error) {
	if n.v.Kind() != reflect.Bool {
		return false, errors.Wrapf(ErrWrongType, "cannot call AsBool on a %v", n.v.Kind())
	}
	return n.v.Bool(), nil
}

func (n *node) AsInt() (int64, error) {
	switch n.v.Kind() {
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64:
		return n.v.Int(), nil
	case
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64:
		return int64(n.v.Uint()), nil
	default:
		return 0, errors.Wrapf(ErrWrongType, "cannot call AsInt on on a %v", n.v.Kind())
	}
}

func (n *node) AsFloat() (float64, error) { return n.v.Float(), nil }
func (n *node) AsBytes() ([]byte, error)  { return n.v.Bytes(), nil }
func (n *node) AsString() (string, error) { return n.v.String(), nil }

func (n *node) AsLink() (datamodel.Link, error) {
	switch n.v.Kind() {
	case reflect.String:
		c, err := cid.Decode(n.v.String())
		if err != nil {
			return nil, err
		}
		return cidlink.Link{Cid: c}, nil
	case reflect.Slice, reflect.Array:
		if n.v.Type().Elem().Kind() != reflect.Uint8 {
			return nil, errors.Wrapf(ErrWrongType, "only []byte or string can be converted to a link")
		}
		c, err := cid.Cast(n.v.Bytes())
		if err != nil {
			return nil, err
		}
		return cidlink.Link{Cid: c}, nil
	}
	switch n.v.Type() {
	case goTypeCid:
		c, ok := n.v.Interface().(cid.Cid)
		if !ok {
			return nil, errors.New("not a cid.Cid")
		}
		return cidlink.Link{Cid: c}, nil
	case goTypeCidPtr:
		c, ok := n.v.Interface().(*cid.Cid)
		if !ok {
			return nil, errors.New("not a *cid.Cid")
		}
		if c == nil {
			return nil, errors.New("cannot convert nil *cid.Cid to Link")
		}
		return cidlink.Link{Cid: *c}, nil
	case goTypeCidLink:
		return n.v.Interface().(cidlink.Link), nil
	default:
		return nil, errors.Wrapf(ErrWrongType, "cannot call AsLink on a \"%v\"", n.v.Kind())
	}
}

func (n *node) Prototype() datamodel.NodePrototype {
	panic("Prototype not implemented")
}

type structNode struct {
	node
	typ   reflect.Type
	pairs []kv
}

func (sn *structNode) Length() int64 {
	return int64(len(sn.pairs))
}

func (sn *structNode) MapIterator() datamodel.MapIterator {
	return &structMapIterator{
		node: sn,
	}
}

func (sn *structNode) LookupByString(key string) (datamodel.Node, error) {
	// TODO change this to a map access
	for _, pair := range sn.pairs {
		k, err := pair.key.AsString()
		if err != nil {
			continue
		}
		if k == key {
			return pair.val, nil
		}
	}
	return nil, errors.Errorf("could not find %q", key)
}

type structMapIterator struct {
	node *structNode
	ix   int
}

func (mi *structMapIterator) Done() bool {
	return mi.ix >= len(mi.node.pairs)
}

func (mi *structMapIterator) Next() (key datamodel.Node, value datamodel.Node, err error) {
	pair := mi.node.pairs[mi.ix]
	mi.ix++
	return pair.key, pair.val, nil
}

type kv struct {
	key, val datamodel.Node
}

type nullNode struct {
	*node // TODO its a little dangerous to use a pointer here
}

func (nn *nullNode) IsNull() bool         { return true }
func (nn *nullNode) Kind() datamodel.Kind { return datamodel.Kind_Null }

func buildPairs(v reflect.Value, typ reflect.Type) []kv {
	n := v.NumField()
	pairs := make([]kv, 0, n)
	for i := 0; i < n; i++ {
		sf := typ.Field(i)
		field := v.Field(i)
		var value datamodel.Node
		isNullPtr := false
		switch field.Kind() {
		case reflect.Pointer:
			if field.IsZero() {
				isNullPtr = true
			} else {
				field = field.Elem()
			}
			if field.Kind() == reflect.Struct {
				typ := field.Type()
				value = &structNode{node: node{v: field}, typ: typ, pairs: buildPairs(field, typ)}
			} else {
				value = &node{v: field}
			}
		case reflect.Interface:
			field = field.Elem()
			value = &node{v: field}
		case reflect.Struct:
			typ := field.Type()
			value = &structNode{node: node{v: field}, typ: typ, pairs: buildPairs(field, typ)}
		default:
			value = &node{v: field}
		}
		name := sf.Name

		rawTag, ok := sf.Tag.Lookup("cbor")
		if ok {
			opts, err := parseTag(rawTag)
			if err != nil {
				// TODO handle this error
				continue
			}
			if len(opts.name) > 0 {
				name = opts.name
			}
			if opts.omitempty && (isNullPtr || field.IsZero()) {
				continue
			}
		}
		if isNullPtr {
			pairs = append(pairs, kv{
				key: &node{v: reflect.ValueOf(name)},
				val: &nullNode{},
			})
		} else {
			pairs = append(pairs, kv{
				key: &node{v: reflect.ValueOf(name)},
				val: value,
			})
		}
	}
	return pairs
}

type mapIterator struct {
	iter *reflect.MapIter
	done bool
	len  int
	ix   int
}

func (mi *mapIterator) Done() bool {
	return mi.ix == mi.len
}

func (mi *mapIterator) Next() (key datamodel.Node, value datamodel.Node, err error) {
	if mi.done {
		return nil, nil, errors.New("map iterator is finished")
	}
	mi.done = !mi.iter.Next()
	key = &node{v: mi.iter.Key()}
	v := mi.iter.Value()
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	value = &node{v: v}
	mi.ix++
	return key, value, nil
}
