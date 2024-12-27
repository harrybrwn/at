package didcache

import (
	"context"
	"log/slog"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/indigo/did"

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
	return nil, nil
}

func (d *Directory) LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error) {
	cache, err := d.cache.CachedDoc(ctx, did.String())
	if err == nil && cache != nil {
		ident := identity.ParseIdentity(atp.ConvertDidDoc(&cache.Doc))
		return &ident, nil
	}
	res, err := d.dir.LookupDID(ctx, did)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (d *Directory) Lookup(ctx context.Context, i syntax.AtIdentifier) (*identity.Identity, error) {
	return nil, nil
}

// Flushes any cache of the indicated identifier. If directory is not using caching, can ignore this.
func (d *Directory) Purge(ctx context.Context, i syntax.AtIdentifier) error {
	if i.IsHandle() {
		// GET DID
		handle, err := i.AsHandle()
		if err != nil {
			return err
		}
		did, err := d.handles.ResolveHandle(ctx, handle.String())
		if err != nil {
			return err
		}
		err = d.cache.ClearHandle(ctx, handle.String())
		if err != nil {
			return err
		}
		return d.cache.ClearEntry(ctx, did.String())
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
	if err == nil {
		if !cached.Expired {
			return &cached.Doc, nil
		}
		err = r.cache.ClearEntry(ctx, didstr)
		if err != nil {
			slog.Error("failed to clear cache for did doc",
				"did", didstr)
		}
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
	if err == nil {
		if !cached.Expired {
			return syntax.ParseDID(cached.DID)
		}
		err = hr.cache.ClearHandle(ctx, handle)
		if err != nil {
			slog.Error("failed to clear cache for handle",
				"handle", handle)
		}
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
