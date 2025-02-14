package didcache

import (
	"context"
	"log/slog"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/indigo/did"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/atp"
)

type Directory struct {
	dir      identity.Directory
	resolver did.Resolver
	handles  atp.HandleResolver
	cache    *DIDCache
}

func NewDirectory(
	dir identity.Directory,
	resolver did.Resolver,
	handles atp.HandleResolver,
	cache *DIDCache,
) *Directory {
	return &Directory{
		dir:      dir,
		resolver: resolver,
		handles:  NewHandleResolver(handles, cache),
		cache:    cache,
	}
}

func (d *Directory) LookupHandle(ctx context.Context, h syntax.Handle) (*identity.Identity, error) {
	did, err := d.handles.ResolveHandle(ctx, h.Normalize().String())
	if err != nil {
		return nil, err
	}
	_ = did
	return d.LookupDID(ctx, did)
}

func (d *Directory) LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error) {
	didstr := did.String()
	cache, err := d.cache.CachedDoc(ctx, didstr)
	if err == nil && !cache.Expired {
		ident := identity.ParseIdentity(atp.ConvertDidDoc(&cache.Doc))
		return &ident, nil
	}
	doc, err := d.resolver.GetDocument(ctx, didstr)
	if err != nil {
		return nil, err
	}
	ident := identity.ParseIdentity(atp.ConvertDidDoc(doc))
	err = d.cache.CacheDoc(ctx, didstr, doc, nil)
	return &ident, err
}

func (d *Directory) Lookup(ctx context.Context, i syntax.AtIdentifier) (*identity.Identity, error) {
	if i.IsDID() {
		did, err := i.AsDID()
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return d.LookupDID(ctx, did)
	} else if i.IsHandle() {
		handle, err := i.AsHandle()
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return d.LookupHandle(ctx, handle)
	}
	return nil, errors.Errorf("invalid AT Identifier: %#v", i)
}

// Flushes any cache of the indicated identifier. If directory is not using caching, can ignore this.
func (d *Directory) Purge(ctx context.Context, i syntax.AtIdentifier) error {
	if i.IsHandle() {
		handle, err := i.AsHandle()
		if err != nil {
			return err
		}
		// If the DID is cached then delete it
		didresult, err := d.cache.GetDID(ctx, handle.String())
		if err == nil {
			err = d.cache.ClearEntry(ctx, didresult.DID)
			if err != nil {
				return err
			}
		}
		err = d.cache.ClearHandle(ctx, handle.String())
		if err != nil {
			return err
		}
		return nil
	} else if i.IsDID() {
		did, err := i.AsDID()
		if err != nil {
			return err
		}
		return d.cache.ClearEntry(ctx, did.String())
	}
	return nil
}

type Resolver struct {
	resolver did.Resolver
	cache    *DIDCache
}

func NewDIDResolver(resolver did.Resolver, cache *DIDCache) *Resolver {
	return &Resolver{resolver: resolver, cache: cache}
}

func (r *Resolver) FlushCacheFor(didstr string) {
	err := r.cache.ClearEntry(context.Background(), didstr)
	if err != nil {
		slog.Warn("failed to flush cache for did", "did", didstr, "error", err)
	}
}

func (r *Resolver) GetDocument(ctx context.Context, didstr string) (*did.Document, error) {
	cached, err := r.cache.CachedDoc(ctx, didstr)
	if err == nil && !cached.Expired {
		return &cached.Doc, nil
	}
	doc, err := r.resolver.GetDocument(ctx, didstr)
	if err != nil {
		return nil, err
	}
	return doc, r.cache.CacheDoc(ctx, didstr, doc, nil)
}

type HandleResolver struct {
	resolver atp.HandleResolver
	cache    *DIDCache
}

func NewHandleResolver(resolver atp.HandleResolver, cache *DIDCache) *HandleResolver {
	return &HandleResolver{
		resolver: resolver,
		cache:    cache,
	}
}

func (hr *HandleResolver) ResolveHandle(ctx context.Context, handle string) (syntax.DID, error) {
	cached, err := hr.cache.GetDID(ctx, handle)
	if err == nil && !cached.Expired {
		return syntax.ParseDID(cached.DID)
	}
	res, err := hr.resolver.ResolveHandle(ctx, handle)
	if err != nil {
		return "", err
	}
	// double check that the did is valid
	did, err := syntax.ParseDID(res.String())
	if err != nil {
		return "", err
	}
	return res, hr.cache.StoreDID(ctx, handle, did.String())
}
