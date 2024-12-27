package atp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/indigo/did"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
)

type DidResolver interface {
	did.Resolver
	HandleResolver
}

type Resolver struct {
	HandleResolver
	PlcURL     *url.URL
	HttpClient *http.Client
}

func (r *Resolver) GetDocument(ctx context.Context, didstr string) (*did.Document, error) {
	var doc did.Document
	did, err := syntax.ParseDID(didstr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	switch did.Method() {
	case "web":
		return &doc, r.resolveDidWeb(ctx, did, &doc)
	case "plc":
		return &doc, r.resolveDidPlc(ctx, did, &doc)
	default:
		return nil, errors.Errorf("unknown did method %q", did.Method())
	}
}

func (r *Resolver) FlushCacheFor(string) {}

func (r *Resolver) LookupHandle(ctx context.Context, h syntax.Handle) (*identity.Identity, error) {
	h = h.Normalize()
	did, err := r.ResolveHandle(ctx, h.String())
	if err != nil {
		return nil, err
	}
	doc, err := r.ResolveDID(ctx, did)
	if err != nil {
		return nil, err
	}
	ident := identity.ParseIdentity(doc)
	declared, err := ident.DeclaredHandle()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if declared != h {
		return &ident, errors.Wrapf(identity.ErrHandleMismatch, "%q != %q", declared, h)
	}
	ident.Handle = declared
	return &ident, nil
}

func (r *Resolver) LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error) {
	doc, err := r.ResolveDID(ctx, did)
	if err != nil {
		return nil, err
	}
	ident := identity.ParseIdentity(doc)
	if ident.DID != did {
		return nil, errors.WithStack(identity.ErrDIDResolutionFailed)
	}
	declared, err := ident.DeclaredHandle()
	if errors.Is(err, identity.ErrHandleNotDeclared) {
		ident.Handle = syntax.HandleInvalid
	} else if err != nil {
		return nil, errors.WithStack(err)
	} else {
		// if a handle was declared, resolve it
		resolvedDID, err := r.ResolveHandle(ctx, string(declared))
		if err != nil {
			if errors.Is(err, identity.ErrHandleNotFound) || errors.Is(err, identity.ErrHandleResolutionFailed) {
				ident.Handle = syntax.HandleInvalid
			} else {
				return nil, err
			}
		} else if resolvedDID != did {
			ident.Handle = syntax.HandleInvalid
		} else {
			ident.Handle = declared
		}
	}
	return &ident, nil
}

func (r *Resolver) Lookup(ctx context.Context, i syntax.AtIdentifier) (*identity.Identity, error) {
	if i.IsDID() {
		did, err := i.AsDID()
		if err != nil {
			return nil, err
		}
		return r.LookupDID(ctx, did)
	} else if i.IsHandle() {
		handle, err := i.AsHandle()
		if err != nil {
			return nil, err
		}
		return r.LookupHandle(ctx, handle)
	} else {
		return nil, errors.New("invalid at identifier")
	}
}

// Flushes any cache of the indicated identifier. If directory is not using caching, can ignore this.
func (r *Resolver) Purge(ctx context.Context, i syntax.AtIdentifier) error {
	return nil
}

var (
	_ did.Resolver       = (*Resolver)(nil)
	_ identity.Directory = (*Resolver)(nil)
)

func (r *Resolver) ResolveDID(ctx context.Context, did syntax.DID) (*identity.DIDDocument, error) {
	var doc identity.DIDDocument
	switch did.Method() {
	case "web":
		return &doc, r.resolveDidWeb(ctx, did, &doc)
	case "plc":
		return &doc, r.resolveDidPlc(ctx, did, &doc)
	default:
		return nil, errors.Errorf("unknown did method %q", did.Method())
	}
}

func (r *Resolver) resolveDidWeb(ctx context.Context, did syntax.DID, dst any) error {
	hostname := did.Identifier()
	handle, err := syntax.ParseHandle(hostname)
	if err != nil {
		return errors.Errorf("did:web identifier not a simple hostname: %s", hostname)
	}
	if !handle.AllowedTLD() {
		return errors.Errorf("did:web hostname has disallowed TLD: %s", hostname)
	}
	u := url.URL{Scheme: "https", Host: hostname, Path: "/.well-known/did.json"}
	req := http.Request{Method: "GET", Host: hostname, URL: &u}
	res, err := r.HttpClient.Do(req.WithContext(ctx))
	if err != nil {
		return errors.WithStack(err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return errors.Wrap(identity.ErrDIDNotFound, "did:web HTTP status 404")
	}
	if res.StatusCode != http.StatusOK {
		return errors.Wrapf(identity.ErrDIDResolutionFailed, "did:web HTTP status %d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
		err = errors.Wrap(identity.ErrDIDResolutionFailed, err.Error())
		return errors.Wrap(err, "failed to parse JSON DID document parse")
	}
	return nil
}

func (r *Resolver) resolveDidPlc(ctx context.Context, did syntax.DID, dst any) error {
	u := *r.PlcURL
	u.Path = filepath.Join("/", did.String())
	req := http.Request{
		Method: "GET",
		Host:   u.Host,
		URL:    &u,
	}
	res, err := r.HttpClient.Do(req.WithContext(ctx))
	if err != nil {
		return errors.WithStack(err)
	}
	defer res.Body.Close()
	if err = json.NewDecoder(res.Body).Decode(dst); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func NewDNSConfig(host string, port int) *dns.ClientConfig {
	return &dns.ClientConfig{
		Servers: []string{host},
		Port:    strconv.FormatInt(int64(port), 10),
	}
}
