package xrpc

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/pkg/errors"
)

type Client struct {
	Client     *http.Client
	Insecure   bool
	Host       string
	AdminToken *string
}

func NewClient(host string) *Client {
	return &Client{
		Client:   http.DefaultClient,
		Insecure: false,
		Host:     host,
	}
}

func (c *Client) Query(ctx context.Context, ns string, q url.Values) (io.ReadCloser, error) {
	res, err := c.do(ctx, Query, ns, q, nil)
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

func (c *Client) Procedure(ctx context.Context, ns string, q url.Values, body io.Reader) (io.ReadCloser, error) {
	res, err := c.do(ctx, Query, ns, q, nil)
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

func (c *Client) Subscription(ctx context.Context) {}

func (c *Client) url(path string, q url.Values) *url.URL {
	u := url.URL{
		Scheme: "https",
		Host:   c.Host,
	}
	if c.Insecure {
		u.Scheme = "http"
	}
	u.Path = filepath.Join("/xrpc", path)
	if q != nil {
		u.RawQuery = q.Encode()
	}
	return &u
}

func (c *Client) do(ctx context.Context, t RequestType, ns string, q url.Values, body io.Reader) (*http.Response, error) {
	u := c.url(ns, q)
	req := http.Request{
		Host:   u.Host,
		URL:    u,
		Header: make(http.Header),
		Body:   io.NopCloser(body),
	}
	switch t {
	case Query:
		req.Method = "GET"
	case Procedure:
		req.Method = "POST"
	}
	if c.AdminToken != nil && (strings.HasPrefix(ns, "com.atproto.admin.") ||
		strings.HasPrefix(ns, "tools.ozone.") ||
		ns == "com.atproto.server.createInviteCode" ||
		ns == "com.atproto.server.createInviteCodes") {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+*c.AdminToken)))
	}
	res, err := c.Client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return res, err
}

type RequestBuilder interface {
	Build() (*http.Request, error)
	Err() error
	Type(RequestType) RequestBuilder
	Insecure() RequestBuilder
	Host(string) RequestBuilder
	NSID(syntax.NSID) RequestBuilder
	Query(url.Values) RequestBuilder
	Body(io.Reader) RequestBuilder
}

type requestBuilder struct {
	r    *http.Request
	err  error
	nsid syntax.NSID
}

func NewReqBuilder() *requestBuilder {
	rb := requestBuilder{
		r: &http.Request{
			Method: "GET",
			URL: &url.URL{
				Scheme: "https",
			},
			Header: make(http.Header),
		},
	}
	return &rb
}

func (rb *requestBuilder) Build() (*http.Request, error) {
	if rb.r == nil {
		rb.r = new(http.Request)
	}
	return rb.r, rb.Err()
}

func (rb *requestBuilder) Err() error { return rb.err }

func (rb *requestBuilder) Type(t RequestType) RequestBuilder {
	switch t {
	case Query:
		rb.r.Method = "GET"
	case Procedure:
		rb.r.Method = "POST"
	case Subscription:
		rb.r.Method = ""
	default:
		if rb.err != nil {
			rb.err = errors.Wrapf(rb.err, "invalid request type %d", t)
		} else {
			rb.err = errors.New("invalid request type")
		}
	}
	return rb
}

func (rb *requestBuilder) NSID(nsid syntax.NSID) RequestBuilder {
	rb.nsid = nsid
	rb.r.URL.Path = path.Join("/xrpc", nsid.String())
	return rb
}

func (rb *requestBuilder) Insecure() RequestBuilder {
	rb.r.URL.Scheme = "http"
	return rb
}

func (rb *requestBuilder) Host(host string) RequestBuilder {
	rb.r.URL.Host = host
	rb.r.Host = host
	return rb
}

func (rb *requestBuilder) Query(v url.Values) RequestBuilder {
	rb.r.URL.RawQuery = v.Encode()
	return rb
}

func (rb *requestBuilder) Body(r io.Reader) RequestBuilder {
	rb.r.Body = io.NopCloser(r)
	return rb
}
