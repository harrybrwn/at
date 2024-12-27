//go:build !openssl

package accountstore

import (
	"github.com/pkg/errors"
	"golang.org/x/crypto/scrypt"
)

func scryptKey(password string, salt []byte) ([]byte, error) {
	// For syncing with the node implementation see:
	// https://nodejs.org/api/crypto.html#cryptoscryptpassword-salt-keylen-options-callback
	//
	// Node's default parameters: https://github.com/nodejs/node/blob/main/lib/internal/crypto/scrypt.js#L33-L38
	dk, err := scrypt.Key(
		[]byte(password),
		salt,
		1<<14, // N (cost). Recommended is 1<<15 but 1<<14 is node's default parameter value.
		8,     // r. 8 is node's default parameter value.
		1,     // p. 1 is node's default parameter value.
		64,    // keylen (64 is what the node implementation @atproto/pds uses)
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return dk, nil
}
