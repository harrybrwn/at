package cbor

type Unmarshaler interface {
	Unmarshal(b []byte, dst any) error
}

type Marshaler interface {
	Marshal(v any) ([]byte, error)
}

type MarshalFunc func(any) ([]byte, error)

func (f MarshalFunc) Marsha(v any) ([]byte, error) { return f(v) }

type UnmarshalFunc func([]byte, any) error

func (f UnmarshalFunc) Unmarshal(b []byte, dst any) error { return f(b, dst) }
