package accountstore

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"github.com/pkg/errors"
)

var (
	ErrInvalidPassword = errors.New("invalid password")
	ErrWrongPassword   = errors.New("wrong password")
)

func hashPassword(pw string) ([]byte, error) {
	salt, err := genRandomBytes(16)
	if err != nil {
		return nil, err
	}
	return hashPasswordWithSalt(pw, salt)
}

func hashAppPassword(did string, password []byte) ([]byte, error) {
	salt := hexenc(sha256sum([]byte(did))[:16])
	return hashPasswordWithSalt(string(password), salt)
}

func verifyPassword(password string, storedhash []byte) error {
	salt, extractedHash, err := extractHashSalt(storedhash)
	if err != nil {
		return err
	}
	hash, err := scryptKey(password, salt)
	if err != nil {
		return err
	}
	if !bytes.Equal(hash, extractedHash) {
		return ErrWrongPassword
	}
	return nil
}

func hashPasswordWithSalt(password string, salt []byte) ([]byte, error) {
	dk, err := scryptKey(password, salt)
	if err != nil {
		return nil, err
	}
	hash := bytes.Join([][]byte{
		salt,
		hexenc(dk),
	}, []byte{':'})
	return hash, nil
}

func extractHashSalt(storedhash []byte) (salt, hash []byte, err error) {
	parts := bytes.SplitN(storedhash, []byte{':'}, 2)
	if len(parts) < 2 {
		return nil, nil, errors.New("invalid password hash format")
	}
	salt = parts[0]
	hash, err = hexdec(parts[1])
	if err != nil {
		return nil, nil, err
	}
	return salt, hash, nil
}

func genRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return b, nil
}

func hexenc(b []byte) []byte {
	res := make([]byte, hex.EncodedLen(len(b)))
	hex.Encode(res, b)
	return res
}

func hexdec(b []byte) ([]byte, error) {
	res := make([]byte, hex.DecodedLen(len(b)))
	_, err := hex.Decode(res, b)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return res, nil
}

func sha256sum(b []byte) []byte {
	h := sha256.New()
	_, _ = h.Write(b)
	return h.Sum(nil)
}
