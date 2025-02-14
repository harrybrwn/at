package auth

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/did"
	"github.com/matryer/is"
)

var jwtKey = []byte("fe62fcf606785c916f265548c39a3628")

func TestAccessToken(t *testing.T) {
	is := is.New(t)
	token, err := CreateAccessToken(&CreateTokenOpts{
		JWTKey: jwtKey,
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

func TestRefreshTokenMiddleware(t *testing.T) {
	is := is.New(t)
	_, refresh, err := CreateTokens(&CreateTokenOpts{
		JWTKey: jwtKey,
		DID:    "did:plc:ar7c4by46qjdydhdevvrndac",
	})
	is.NoErr(err)
	h := RefreshTokenOnly(&Opts{
		Logger:    slog.Default(),
		JWTSecret: jwtKey,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", refresh))
	h.ServeHTTP(rec, req)
	is.Equal(rec.Code, 200)
}

func TestPublicKeyFromDIDDoc(t *testing.T) {
	rawDidDoc := `{
        "@context": ["https://www.w3.org/ns/did/v1","https://w3id.org/security/multikey/v1","https://w3id.org/security/suites/secp256k1-2019/v1"],
        "id": "did:plc:nsu4iq7726acidyqpha2zuk3",
        "alsoKnownAs": ["at://harry.test"],
        "verificationMethod": [{"id":"did:plc:nsu4iq7726acidyqpha2zuk3#atproto",
        "type": "Multikey",
        "controller": "did:plc:nsu4iq7726acidyqpha2zuk3",
        "publicKeyMultibase": "zQ3shZbwwMVNVtB2Aa1f2BGDoBNq3sQVLcp5iJRQj99xMjKna"}],
        "service": [{"id":"#atproto_pds","type":"AtprotoPersonalDataServer","serviceEndpoint":"http://localhost:3000"}]
    }`
	var doc did.Document
	err := json.Unmarshal([]byte(rawDidDoc), &doc)
	if err != nil {
		t.Fatal(err)
	}
	key, err := publicKeyFromDidDoc(&doc, "atproto")
	if err != nil {
		t.Fatal(err)
	}
	vm, err := getVerificationMethod(&doc, "atproto")
	if err != nil {
		t.Fatal(err)
	}
	if key.Multibase() != *vm.PublicKeyMultibase {
		t.Error("parsed multibase should be the same as the input multibase")
	}
}
