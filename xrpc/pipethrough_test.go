package xrpc

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/matryer/is"
)

func TestPipethrough(t *testing.T) {
	is := is.New(t)
	p := NewPipethrough("api.bsky.app")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		"GET",
		"https://localhost/xrpc/app.bsky.actor.getProfile?actor=did%3Aplc%3Akzvsijt4365vidgqv7o6wksi",
		nil,
	)
	p.ServeHTTP(rec, req)
	is.Equal(rec.Code, 200)
	is.Equal(rec.Header().Get("Content-Type"), "application/json; charset=utf-8")
	is.Equal(rec.Header().Get("Access-Control-Allow-Origin"), "*")
	is.True(len(rec.Header().Get("Etag")) > 0)
	is.True(len(rec.Header().Get("Date")) > 0)
	body := make(map[string]any)
	is.NoErr(json.Unmarshal(rec.Body.Bytes(), &body))
	is.Equal(body["did"], "did:plc:kzvsijt4365vidgqv7o6wksi")
	is.Equal(body["handle"], "hrry.me")
	is.Equal(body["displayName"], "Harry Brown")
}
