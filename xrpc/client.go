package xrpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/pkg/errors"
)

type Client struct {
	Client     *http.Client
	Insecure   bool
	Host       string
	AdminToken *string
	Auth       *Auth
}

type ClientOption func(*Client)

func NewClient(opts ...ClientOption) *Client {
	c := Client{
		Client:   http.DefaultClient,
		Insecure: false,
	}
	for _, o := range opts {
		o(&c)
	}
	return &c
}

func WithAdminPassword(pw string) ClientOption    { return func(c *Client) { c.AdminToken = &pw } }
func WithInsecure() ClientOption                  { return func(c *Client) { c.Insecure = true } }
func WithJwt(token string) ClientOption           { return func(c *Client) { c.Auth = &Auth{AccessJwt: token} } }
func WithHost(host string) ClientOption           { return func(c *Client) { c.Host = host } }
func WithClient(client *http.Client) ClientOption { return func(c *Client) { c.Client = client } }

func WithEnv() ClientOption {
	return func(c *Client) {
		if v, ok := os.LookupEnv("PDS_ADMIN_PASSWORD"); ok {
			c.AdminToken = &v
		}
		if v, ok := os.LookupEnv("PDS_CLIENT_INSECURE"); ok {
			insecure, err := strconv.ParseBool(v)
			if err == nil {
				c.Insecure = insecure
			}
		}
		if v, ok := os.LookupEnv("PDS_CLIENT_JWT"); ok {
			c.Auth = &Auth{AccessJwt: v}
		}
	}
}

func WithURL(uri string) ClientOption {
	u, err := url.Parse(uri)
	if err != nil {
		slog.Error("Failed to parse url in xrpc.WithURL", "error", err)
		return func(c *Client) {}
	}
	return func(c *Client) {
		c.Host = u.Host
		if u.Scheme == "http" {
			c.Insecure = true
		}
		if u.User != nil {
			pw, ok := u.User.Password()
			switch u.User.Username() {
			case "admin":
				if ok {
					c.AdminToken = &pw
				}
			default:
				c.Auth = &Auth{
					Handle:    u.User.Username(),
					AccessJwt: pw,
				}
			}
		}
	}
}

func (c *Client) Ping(ctx context.Context) error {
	u := url.URL{Scheme: "https", Host: c.Host, Path: "/xrpc/_health"}
	if c.Insecure {
		u.Scheme = "http"
	}
	req := http.Request{Method: "GET", Host: c.Host, URL: &u}
	res, err := c.Client.Do(&req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errors.Errorf("invalid status code %q", res.Status)
	}
	return nil
}

type Request struct {
	Type        RequestType
	ContentType string
	NSID        string
	Params      url.Values
	Body        io.Reader
}

func (c *Client) Query(ctx context.Context, req *Request) (io.ReadCloser, error) {
	req.Type = Query
	res, err := c.do(ctx, Query, req.ContentType, req.NSID, req.Params, req.Body)
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

func (c *Client) Procedure(ctx context.Context, req *Request) (io.ReadCloser, error) {
	req.Type = Procedure
	res, err := c.do(ctx, Procedure, req.ContentType, req.NSID, req.Params, req.Body)
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

func (c *Client) do(ctx context.Context, t RequestType, contentType, ns string, q url.Values, body io.Reader) (*http.Response, error) {
	u := c.url(ns, q)
	req := http.Request{
		Host:   u.Host,
		URL:    u,
		Header: make(http.Header),
	}
	if body != nil {
		req.Body = io.NopCloser(body)
	}
	switch t {
	case Query:
		req.Method = "GET"
	case Procedure:
		req.Method = "POST"
	}
	if len(contentType) > 0 {
		req.Header.Set("Content-Type", contentType)
	}
	if c.AdminToken != nil &&
		(strings.HasPrefix(ns, "com.atproto.admin.") ||
			strings.HasPrefix(ns, "tools.ozone.") ||
			ns == "com.atproto.server.createInviteCode" ||
			ns == "com.atproto.server.createInviteCodes") {
		auth := base64.StdEncoding.EncodeToString([]byte("admin:" + *c.AdminToken))
		req.Header.Set(
			"Authorization",
			"Basic "+auth,
		)
	} else if c.Auth != nil {
		// TODO add jwt token
		req.Header.Set(
			"Authorization",
			"Bearer "+c.Auth.AccessJwt,
		)
	}

	res, err := c.Client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if res.StatusCode >= 400 {
		e := ErrorResponse{Status: res.StatusCode}
		err = json.NewDecoder(res.Body).Decode(&e)
		if err != nil {
			res.Body.Close()
			return nil, e.Wrap(err)
		}
		if err = res.Body.Close(); err != nil {
			return nil, e.Wrap(err)
		}
		return nil, errors.WithStack(&e)
	}
	return res, nil
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

func RequestFromHttp(r *http.Request) *Request {
	req := Request{
		ContentType: r.Header.Get("Content-Type"),
	}
	if r.Body != nil {
		req.Body = r.Body
	}
	switch r.Method {
	case "GET":
		req.Type = Query
	case "POST":
		req.Type = Procedure
	}
	if r.URL != nil {
		_, ns := filepath.Split(r.URL.Path)
		req.NSID = ns
		q := r.URL.Query()
		if len(q) > 0 {
			req.Params = q
		}
	}
	return &req
}
