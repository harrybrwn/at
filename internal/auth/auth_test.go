package auth

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestAccessToken(t *testing.T) {
	is := is.New(t)
	token, err := CreateAccessToken(&CreateTokenOpts{
		JWTKey: []byte("fe62fcf606785c916f265548c39a3628"),
		DID:    "did:plc:ar7c4by46qjdydhdevvrndac",
	})
	is.NoErr(err)
	is.True(strings.IndexByte(token, '.') > 0)
}

func TestGetRefreshTokenID(t *testing.T) {
	id, err := GetRefreshTokenID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) == 0 {
		t.Fatal("id should not be zero length")
	}
	if id[len(id)-1] == '=' {
		t.Error("getRefreshTokenID should trim all base64 padding off the end")
	}
}
