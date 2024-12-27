package dagcbor

import (
	"bytes"
	"io"
	"reflect"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/ipld/go-ipld-prime/schema"
	schemadmt "github.com/ipld/go-ipld-prime/schema/dmt"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/internal/ipldutil"
)

var TypeSystem = schemadmt.TypeSystem

func Register(v any) {
	ipldutil.InferSchema(reflect.TypeOf(v), &TypeSystem)
}

func Unmarshal(b []byte, dst any) (err error) {
	switch v := dst.(type) {
	case interface{ UnmarshalCBOR([]byte) error }:
		err = v.UnmarshalCBOR(b)
	case interface{ UnmarshalCBOR(io.Reader) error }:
		err = v.UnmarshalCBOR(bytes.NewBuffer(b))
	default:
		err = dagcbor.Decode(ipldutil.NodeAssembler(dst), bytes.NewBuffer(b))
	}
	return errors.WithStack(err)
}

func Marshal(val any) ([]byte, error) {
	switch v := val.(type) {
	case interface{ MarshalCBOR() ([]byte, error) }:
		return v.MarshalCBOR()
	case interface{ MarshalCBOR(io.Writer) error }:
		var buf bytes.Buffer
		err := v.MarshalCBOR(&buf)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return buf.Bytes(), nil
	}
	var buf bytes.Buffer
	node := ipldutil.BuildNode(val)
	err := dagcbor.Encode(node, &buf)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return buf.Bytes(), nil
}

func Decode(r io.Reader, dst any) (err error) {
	switch v := dst.(type) {
	case interface{ UnmarshalCBOR([]byte) error }:
		var b []byte
		b, err = io.ReadAll(r)
		if err != nil {
			return errors.WithStack(err)
		}
		err = v.UnmarshalCBOR(b)
	case interface{ UnmarshalCBOR(io.Reader) error }:
		err = v.UnmarshalCBOR(r)
	case interface {
		NodeAssembler() datamodel.NodeAssembler
	}:
		err = errors.WithStack(dagcbor.Decode(v.NodeAssembler(), r))
	default:
		err = dagcbor.Decode(ipldutil.NodeAssembler(dst), r)
	}
	return errors.WithStack(err)
}

func Encode(w io.Writer, src any) (err error) {
	switch v := src.(type) {
	case interface{ MarshalCBOR() ([]byte, error) }:
		var b []byte
		b, err = v.MarshalCBOR()
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = w.Write(b)
	case interface{ MarshalCBOR(io.Writer) error }:
		err = v.MarshalCBOR(w)
	case interface{ ToNode() datamodel.Node }:
		err = dagcbor.Encode(v.ToNode(), w)
	default:
		err = dagcbor.Encode(ipldutil.BuildNode(src), w)
	}
	return errors.WithStack(err)
}

// BOOOO don't use this garbage
func MarshalWithSchema(val any, sch schema.Type) ([]byte, error) {
	node := bindnode.Wrap(val, sch)
	return ipld.Encode(node.Representation(), dagcbor.Encode)
}
