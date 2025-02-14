package cid

import (
	"encoding/json"

	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
)

// The same as "github.com/ipfs/go-cid.Cid" except the [UnmarshalJSON] function
// isn't completely ridiculous.
type Cid cid.Cid

func (c *Cid) MarshalJSON() ([]byte, error) {
	s := (*cid.Cid)(c).String()
	return []byte("\"" + s + "\""), nil
}

func (c *Cid) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return errors.WithStack(err)
	}
	cid, err := cid.Decode(s)
	if err != nil {
		return err
	}
	*c = Cid(cid)
	return nil
}

func (c *Cid) String() string {
	return (*cid.Cid)(c).String()
}

func (c *Cid) ByteLen() int {
	return (*cid.Cid)(c).ByteLen()
}

func Decode(v string) (Cid, error) {
	c, err := cid.Decode(v)
	if err != nil {
		return Cid(cid.Undef), err
	}
	return Cid(c), nil
}

func Parse(v any) (Cid, error) {
	c, err := cid.Parse(v)
	if err != nil {
		return Cid(cid.Undef), err
	}
	return Cid(c), nil
}
