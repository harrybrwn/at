package ipldutil

import (
	"reflect"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/pkg/errors"
)

func NodeAssembler(dst any) datamodel.NodeAssembler {
	v := reflect.ValueOf(dst)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	return &simpleValueAssembler{v}
}

type simpleValueAssembler struct {
	v reflect.Value
}

func (a *simpleValueAssembler) ptrGuard() reflect.Value {
	v := a.v
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	return v
}

func (a *simpleValueAssembler) BeginMap(sizeHint int64) (datamodel.MapAssembler, error) {
	v := a.ptrGuard()
	switch v.Kind() {
	case reflect.Map:
		return &mapAssembler{
			v: v,
			m: make(map[string]reflect.Value),
		}, nil
	case reflect.Interface:
		return newEmptyMapAssembler(), nil
	case reflect.Struct:
		ma := valueStructAssembler{v: v}
		return &ma, ma.build()
	default:
		return nil, errors.Errorf("cannot assemble map with kind %v", v.Kind())
	}
}

func (a *simpleValueAssembler) BeginList(sizeHint int64) (datamodel.ListAssembler, error) {
	return &listAssembler{
		dst:    a.v,
		values: make([]reflect.Value, 0, sizeHint),
	}, nil
}

func (a *simpleValueAssembler) AssignNull() error {
	a.v.Set(reflect.Zero(a.v.Type()))
	return nil
}

func (a *simpleValueAssembler) AssignBool(b bool) error {
	switch a.v.Kind() {
	case reflect.Pointer:
		a.v.Set(reflect.ValueOf(&b))
	case reflect.Bool:
		a.v.Set(reflect.ValueOf(b))
	default:
		return errors.Wrapf(ErrWrongType, "could not assign bool")
	}
	return nil
}

func (a *simpleValueAssembler) AssignInt(i int64) error {
	v := a.ptrGuard()
	switch v.Kind() {
	case reflect.Int:
		v.Set(reflect.ValueOf(int(i)))
	case reflect.Int8:
		v.Set(reflect.ValueOf(int8(i)))
	case reflect.Int16:
		v.Set(reflect.ValueOf(int16(i)))
	case reflect.Int32:
		v.Set(reflect.ValueOf(int32(i)))
	case reflect.Int64:
		v.Set(reflect.ValueOf(i))
	case reflect.Uint:
		v.Set(reflect.ValueOf(uint(i)))
	case reflect.Uint8:
		v.Set(reflect.ValueOf(uint8(i)))
	case reflect.Uint16:
		v.Set(reflect.ValueOf(uint16(i)))
	case reflect.Uint32:
		v.Set(reflect.ValueOf(uint32(i)))
	case reflect.Uint64:
		v.Set(reflect.ValueOf(uint64(i)))
	case reflect.Interface:
		// underlying type is any
		v.Set(reflect.ValueOf(i))
	default:
		return errors.Wrapf(ErrWrongType, "type %q is not an int", a.v.Kind())
	}
	return nil
}

func (a *simpleValueAssembler) AssignFloat(f float64) error {
	v := a.ptrGuard()
	switch v.Kind() {
	case reflect.Float32:
		v.Set(reflect.ValueOf(float32(f)))
	case reflect.Float64:
		v.Set(reflect.ValueOf(f))
	default:
		return errors.Wrapf(ErrWrongType, "could not assign float")
	}
	return nil
}

func (a *simpleValueAssembler) AssignString(s string) error {
	if a.v.Kind() == reflect.Pointer {
		a.v.Set(reflect.ValueOf(&s))
		return nil
	}
	if a.v.Kind() == reflect.Interface && a.v.CanAddr() {
		a.v.Set(reflect.ValueOf((any)(s)))
		return nil
	} else if a.v.Kind() != reflect.String {
		return errors.Wrapf(ErrWrongType, "expected type to be %q not %q", reflect.String, a.v.Kind())
	}
	a.v.Set(reflect.ValueOf(s))
	return nil
}

func (a *simpleValueAssembler) AssignBytes(b []byte) error {
	v := a.ptrGuard()
	switch v.Kind() {
	case reflect.Array:
		if v.Type().Elem().Kind() != reflect.Uint8 {
			return errors.Wrapf(ErrWrongType, "could not assign bytes to array")
		}
		if len(b) > v.Len() {
			return errors.Errorf("cannot assign % bytes to [%d]byte", len(b), v.Len())
		}
		for i := 0; i < v.Len(); i++ {
			a.v.Index(i).Set(reflect.ValueOf(b[i]))
		}
		return nil
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.Uint8 {
			return errors.Wrapf(ErrWrongType, "could not assign bytes to slice")
		}
		v.Set(reflect.ValueOf(b))
		return nil
	default:
		return errors.Wrapf(ErrWrongType, "could not assign bytes to unknown type %v", v.Kind())
	}
}

func (a *simpleValueAssembler) AssignLink(l datamodel.Link) error {
	v := a.ptrGuard()
	if !v.IsValid() {
		cid, err := cid.Cast([]byte(l.Binary()))
		if err != nil {
			return err
		}
		a.v = reflect.ValueOf(cid)
		return nil
	}
	switch v.Type() {
	case goTypeCid:
		cid, err := cid.Cast([]byte(l.Binary()))
		if err != nil {
			return err
		}
		v.Set(reflect.ValueOf(cid))
		return nil
	case goTypeCidLink:
		cid, err := cid.Cast([]byte(l.Binary()))
		if err != nil {
			return err
		}
		v.Set(reflect.ValueOf(cidlink.Link{Cid: cid}))
		return nil
	case goTypeSyntaxCID:
		cid, err := syntax.ParseCID(l.String())
		if err != nil {
			return err
		}
		v.Set(reflect.ValueOf(cid))
		return nil
	default:
		if v.Kind() == reflect.Interface && v.IsNil() && v.CanSet() {
			cid, err := cid.Cast([]byte(l.Binary()))
			if err != nil {
				return err
			}
			v.Set(reflect.ValueOf(cid))
			return nil
		}
		return errors.Wrapf(ErrWrongType, "could not assign link to type %v", v.Type())
	}
}

func (a *simpleValueAssembler) AssignNode(n datamodel.Node) error { // if you already have a completely constructed subtree, this method puts the whole thing in place at once.
	switch n.Kind() {
	case datamodel.Kind_Invalid:
		return errors.New("cannot handle invalid node")
	case datamodel.Kind_Null:
		a.ptrGuard().Set(reflect.Zero(a.v.Type()))
		return nil
	case datamodel.Kind_Bool:
		v, err := n.AsBool()
		if err != nil {
			return errors.WithStack(err)
		}
		return a.AssignBool(v)
	case datamodel.Kind_Int:
		v, err := n.AsInt()
		if err != nil {
			return errors.WithStack(err)
		}
		return a.AssignInt(v)
	case datamodel.Kind_Float:
		v, err := n.AsFloat()
		if err != nil {
			return errors.WithStack(err)
		}
		return a.AssignFloat(v)
	case datamodel.Kind_String:
		v, err := n.AsString()
		if err != nil {
			return errors.WithStack(err)
		}
		return a.AssignString(v)
	case datamodel.Kind_Bytes:
		v, err := n.AsBytes()
		if err != nil {
			return errors.WithStack(err)
		}
		return a.AssignBytes(v)
	case datamodel.Kind_Link:
		v, err := n.AsLink()
		if err != nil {
			return errors.WithStack(err)
		}
		return a.AssignLink(v)
	case datamodel.Kind_Map:
		ma, err := a.BeginMap(0)
		if err != nil {
			return err
		}
		it := n.MapIterator()
		for !it.Done() {
			keynode, valnode, err := it.Next()
			if err != nil {
				return err
			}
			// TODO Add a setting to support other key types, for now DAG-CBOR
			// only supports string keys so this is fine.
			key, err := keynode.AsString()
			if err != nil {
				return err
			}
			assembler, err := ma.AssembleEntry(key)
			if err != nil {
				return err
			}
			err = assembler.AssignNode(valnode)
			if err != nil {
				return err
			}
		}
		return ma.Finish()
	case datamodel.Kind_List:
		la, err := a.BeginList(0)
		if err != nil {
			return errors.WithStack(err)
		}
		it := n.ListIterator()
		for !it.Done() {
			_, valnode, err := it.Next()
			if err != nil {
				return errors.WithStack(err)
			}
			err = la.AssembleValue().AssignNode(valnode)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return la.Finish()
	default:
		return errors.New("unknown type")
	}
}

func (a *simpleValueAssembler) Prototype() datamodel.NodePrototype {
	// return nodePrototypeFromKind(a.v)
	return nil
}

var _ datamodel.MapAssembler = (*valueStructAssembler)(nil)

type valueStructAssembler struct {
	v      reflect.Value
	fields map[string]field
}

type field struct {
	sf *reflect.StructField
	v  reflect.Value
}

func (a *valueStructAssembler) build() error {
	a.fields = make(map[string]field)
	typ := a.v.Type()
	var n int
	if a.v.Kind() == reflect.Map {
		n = a.v.Len()
	} else {
		n = a.v.NumField()
	}
	for i := 0; i < n; i++ {
		sf := typ.Field(i)
		v := a.v.Field(i)
		name := sf.Name
		// name = string(unicode.ToLower(rune(name[0]))) + name[1:]
		rawTag, ok := sf.Tag.Lookup("cbor")
		if ok {
			var (
				opts *tagOpt
				err  error
			)
			opts, err = parseTag(rawTag)
			if err != nil {
				return err
			}
			_ = opts
			if len(opts.name) > 0 {
				name = opts.name
			}
		}
		a.fields[name] = field{
			sf: &sf,
			v:  v,
		}
	}
	return nil
}

type tagOpt struct {
	name      string
	omitempty bool
}

func parseTag(raw string) (*tagOpt, error) {
	parts := strings.Split(raw, ",")
	opt := tagOpt{name: parts[0]}
	for _, label := range parts[1:] {
		switch label {
		case "omitempty":
			opt.omitempty = true
		}
	}
	return &opt, nil
}

func getTagOpts(sf *reflect.StructField) (*tagOpt, error) {
	rawTag, ok := sf.Tag.Lookup("cbor")
	if !ok {
		rawTag, ok = sf.Tag.Lookup("cborgen")
	}
	if ok {
		return parseTag(rawTag)
	}
	return new(tagOpt), nil
}

// shortcut combining AssembleKey and AssembleValue into one step; valid when the key is a string kind.
func (a *valueStructAssembler) AssembleEntry(k string) (datamodel.NodeAssembler, error) {
	f := a.fields[k]
	return &simpleValueAssembler{f.v}, nil
}

func (a *valueStructAssembler) Finish() error { return nil }

func (a *valueStructAssembler) KeyPrototype() datamodel.NodePrototype {
	return basicnode.Prototype.String
}

func (a *valueStructAssembler) ValuePrototype(k string) datamodel.NodePrototype {
	f := a.fields[k]
	return nodePrototypeFromKind(f.v)
	// return nil
}

// must be followed by call to AssembleValue.
func (a *valueStructAssembler) AssembleKey() datamodel.NodeAssembler {
	panic("help I don't know how to implement AssembleKey")
}

// must be called immediately after AssembleKey.
func (a *valueStructAssembler) AssembleValue() datamodel.NodeAssembler {
	panic("help I don't know how to implement AssembleValue")
}

func nodePrototypeFromKind(v reflect.Value) datamodel.NodePrototype {
	// TODO finish this
	switch v.Kind() {
	case reflect.Bool:
		return basicnode.Prototype.Bool
	case reflect.String:
		return basicnode.Prototype.String
	case reflect.Array, reflect.Slice:
		if v.Elem().Kind() == reflect.Uint8 {
			return basicnode.Prototype.Bytes
		}
		return basicnode.Prototype.List
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint,
		reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return basicnode.Prototype.Int
	case reflect.Float32, reflect.Float64:
		return basicnode.Prototype.Float
	case reflect.Map:
		return basicnode.Prototype.Map
	}
	return basicnode.Prototype.Any
}

var _ datamodel.ListAssembler = (*listAssembler)(nil)

type listAssembler struct {
	dst    reflect.Value
	values []reflect.Value
}

func (a *listAssembler) AssembleValue() datamodel.NodeAssembler {
	var v reflect.Value
	if a.dst.Kind() == reflect.Interface {
		v = reflect.New(a.dst.Type()).Elem()
	} else {
		v = reflect.New(a.dst.Type().Elem()).Elem()
	}
	// v will act like a pointer, so it's inner value will be changed later on
	// by the simpleValueAssembler that we return which will also alter the
	// value stored in the listAssembler's values slice.
	a.values = append(a.values, v)
	return &simpleValueAssembler{v}
}

func (a *listAssembler) Finish() error {
	tp := a.dst.Type()
	if a.dst.Kind() == reflect.Interface {
		tp = reflect.TypeOf(([]any)(nil))
	}
	slice := reflect.MakeSlice(
		tp,
		len(a.values),
		len(a.values),
	)
	for i, v := range a.values {
		slice.Index(i).Set(v)
	}
	a.dst.Set(slice)
	return nil
}

func (a *listAssembler) ValuePrototype(idx int64) datamodel.NodePrototype {
	return basicnode.Prototype.List
}

type mapAssembler struct {
	v reflect.Value
	m map[string]reflect.Value
}

// shortcut combining AssembleKey and AssembleValue into one step; valid when the key is a string kind.
func (a *mapAssembler) AssembleEntry(k string) (datamodel.NodeAssembler, error) {
	v := reflect.New(a.v.Type().Elem())
	a.m[k] = v
	return &anyAssembler{
		simpleValueAssembler: &simpleValueAssembler{
			v.Elem(),
		},
	}, nil
}

func (a *mapAssembler) Finish() error {
	for k, v := range a.m {
		a.v.SetMapIndex(reflect.ValueOf(k), v.Elem())
	}
	return nil
}

func (a *mapAssembler) KeyPrototype() datamodel.NodePrototype {
	// dag-cbor says all keys should be strings
	return basicnode.Prototype.String
}

func (a *mapAssembler) ValuePrototype(k string) datamodel.NodePrototype {
	v := a.v.MapIndex(reflect.ValueOf(k))
	// TODO this is probably wrong and will cause problems called
	return nodePrototypeFromKind(v)
}

// must be followed by call to AssembleValue.
func (a *mapAssembler) AssembleKey() datamodel.NodeAssembler {
	panic("help I don't know how to implement AssembleKey")
}

// must be called immediately after AssembleKey.
func (a *mapAssembler) AssembleValue() datamodel.NodeAssembler {
	panic("help I don't know how to implement AssembleValue")
}

type anyAssembler struct {
	*simpleValueAssembler
}

func newEmptyMapAssembler() *mapAssembler {
	tp := reflect.TypeOf((map[string]any)(nil))
	return &mapAssembler{
		v: reflect.MakeMap(tp),
		m: make(map[string]reflect.Value),
	}
}

func (aa *anyAssembler) BeginMap(sizeHint int64) (datamodel.MapAssembler, error) {
	tp := reflect.TypeOf((map[string]any)(nil))
	v := reflect.MakeMap(tp)
	aa.v.Set(v)
	ma := mapAssembler{
		v: v,
		m: make(map[string]reflect.Value),
	}
	return &ma, nil
}

func (aa *anyAssembler) BeginList(sizeHint int64) (datamodel.ListAssembler, error) {
	la := listAssembler{
		dst:    aa.v,
		values: make([]reflect.Value, 0, sizeHint),
	}
	return &la, nil
}

func (aa *anyAssembler) AssignString(s string) error {
	if aa.v.IsValid() {
		if aa.v.Kind() == reflect.Interface && aa.v.CanSet() {
			aa.v.Set(reflect.ValueOf(s))
			return nil
		}
		return aa.simpleValueAssembler.AssignString(s)
	}
	aa.v = reflect.ValueOf(s)
	return nil
}

func (aa *anyAssembler) AssignBytes(b []byte) error {
	if aa.v.IsValid() {
		return aa.simpleValueAssembler.AssignBytes(b)
	}
	aa.v = reflect.ValueOf(b)
	return nil
}

func (aa *anyAssembler) AssignLik(l datamodel.Link) error {
	if aa.v.IsValid() {
		return aa.simpleValueAssembler.AssignLink(l)
	}
	aa.v = reflect.ValueOf(cid.Cid{})
	cid, err := cid.Cast([]byte(l.Binary()))
	if err != nil {
		return err
	}
	aa.v.Set(reflect.ValueOf(cid))
	return nil
}

func (aa *anyAssembler) AssignInt(i int64) error {
	if aa.v.IsValid() {
		return aa.simpleValueAssembler.AssignInt(i)
	}
	aa.v = reflect.ValueOf(i)
	return nil
}

func ptr[T any](v T) *T { return &v }
