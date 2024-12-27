package atp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/miekg/dns"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/internal/parallel"
)

type HandleResolver interface {
	ResolveHandle(ctx context.Context, handle string) (syntax.DID, error)
}

type handleResolver struct {
	dnsResolver
	wellKnownResolver
	timeout time.Duration
}

func NewHandleResolver(dnsConf *dns.ClientConfig, httpClient *http.Client, timeout time.Duration) *handleResolver {
	var hr handleResolver
	hr.conf = dnsConf
	hr.client = httpClient
	hr.timeout = timeout
	return &hr
}

func NewDefaultHandleResolver() (*handleResolver, error) {
	dnsConfig, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return nil, err
	}
	if len(dnsConfig.Servers) == 0 {
		return nil, errors.New("no dns servers configured")
	}
	return NewHandleResolver(dnsConfig, http.DefaultClient, time.Second*5), nil
}

func (hr *handleResolver) ResolveHandle(ctx context.Context, handle string) (syntax.DID, error) {
	ctrl := parallel.NewCtrl[string, syntax.DID](&parallel.JobConfig{
		Timeout: hr.timeout,
	})
	return ctrl.FirstOf(
		ctx,
		handle,
		[]parallel.Job[string, syntax.DID]{
			hr.dnsResolver.resolveHandle,
			hr.wellKnownResolver.resolveHandle,
		},
	)
}

type dnsResolver struct {
	conf *dns.ClientConfig
}

func (dr *dnsResolver) resolveHandle(ctx context.Context, handle string) (syntax.DID, error) {
	if !strings.HasPrefix(handle, "_atproto.") {
		handle = fmt.Sprintf("_atproto.%s", handle)
	}
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(handle), dns.TypeTXT)
	m.RecursionDesired = true
	res, _, err := c.ExchangeContext(
		ctx,
		m,
		net.JoinHostPort(dr.conf.Servers[0], dr.conf.Port),
	)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if len(res.Answer) == 0 {
		return "", errors.New("empty answer from dns server")
	}
	for _, answer := range res.Answer {
		ans, ok := answer.(*dns.TXT)
		if !ok {
			return "", errors.New("expected TXT request to return a *dns.TXT type")
		}
		did, found := strings.CutPrefix(ans.Txt[0], "did=")
		if !found {
			continue
		}
		resdid, err := syntax.ParseDID(did)
		return resdid, errors.WithStack(err)
	}
	return "", errors.Errorf("failed to find did from %q", handle)
}

type wellKnownResolver struct {
	client *http.Client
}

func (wkr *wellKnownResolver) resolveHandle(ctx context.Context, handle string) (syntax.DID, error) {
	u := &url.URL{
		Scheme: "http",
		Host:   handle,
		Path:   "/.well-known/atproto-did",
	}
	for i := 0; i < 20; i++ {
		req := http.Request{
			Method: "GET",
			Host:   u.Host,
			URL:    u,
		}
		res, err := wkr.client.Do(req.WithContext(ctx))
		if err != nil {
			return "", errors.WithStack(err)
		}
		switch {
		case res.StatusCode == http.StatusNotFound:
			return "", errors.New(".well-known/atproto-did not found")
		case res.StatusCode >= 300 && res.StatusCode < 400:
			u, err = url.Parse(res.Header.Get("Location"))
			if err != nil {
				return "", errors.WithStack(err)
			}
			continue
		case res.StatusCode >= 500:
			return "", errors.Errorf("server failure %d from %q", res.StatusCode, u.Host)
		}
		var buf bytes.Buffer
		_, err = io.Copy(&buf, res.Body)
		if err != nil {
			res.Body.Close()
			return "", errors.WithStack(err)
		}
		if err = res.Body.Close(); err != nil {
			return "", errors.WithStack(err)
		}
		did, err := syntax.ParseDID(buf.String())
		return did, errors.WithStack(err)
	}
	return "", errors.New("too many redirects")
}
