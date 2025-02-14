package xrpc

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/matryer/is"
)

func TestRequestFromHttp(t *testing.T) {
	type TT struct {
		in  http.Request
		exp Request
	}
	is := is.New(t)
	for _, tt := range []TT{
		{
			*must(http.NewRequest("GET", "/xrpc/com.atproto.identity.resolveHandle", nil)),
			Request{
				Type: Query,
				NSID: "com.atproto.identity.resolveHandle",
			},
		},
		{
			http.Request{
				Method: "POST",
				URL: &url.URL{
					Path:     "/xrpc/com.atproto.repo.createRecord",
					RawQuery: "a=one&b=two",
				},
				Body: io.NopCloser(bytes.NewBuffer([]byte(`{}`))),
			},
			Request{
				Type: Procedure,
				NSID: "com.atproto.repo.createRecord",
				Body: io.NopCloser(bytes.NewBuffer([]byte(`{}`))),
				Params: url.Values{
					"a": []string{"one"},
					"b": []string{"two"},
				},
			},
		},
	} {
		r := RequestFromHttp(&tt.in)
		is.Equal(r.Type, tt.exp.Type)
		is.Equal(r.NSID, tt.exp.NSID)
		is.Equal(r.ContentType, tt.exp.ContentType)
		is.Equal(r.Params, tt.exp.Params)
		if tt.exp.Body != nil {
			is.True(r.Body != nil)
		}
		// is.Equal(*r, tt.exp)
		if r.Type != tt.exp.Type {
			t.Errorf("wrong type; got %v, want %v", r.Type, tt.exp.Type)
		}
	}
}

func must[T any](v T, e error) T {
	if e != nil {
		panic(e)
	}
	return v
}
