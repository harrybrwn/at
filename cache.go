package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

var (
	ErrNoCache      = errors.New("caching is disabled")
	ErrDIDTooLong   = errors.New("DID is too long (2048 chars max)")
	ErrFileTooBig   = errors.New("cached file is too large")
	ErrCacheExpired = errors.New("cached file is too old")
)

const DIDMaxLen = 2 * 1024

type CacheConfig struct{}

func cache(d identity.Directory, cleaner CacheCleaner, dir string) *cacheDirectory {
	memcache := identity.NewCacheDirectory(
		d,
		250_000,
		time.Hour*24,
		time.Minute*2,
		time.Minute*5,
	)
	return &cacheDirectory{
		// inner: d,
		inner: &memcache,
		fileCache: &fileCache{
			dir:      dir,
			lifetime: time.Hour,
			disabled: false,
			cleaner:  cleaner,
			logger:   slog.Default(),
		},
	}
}

type CacheCleaner interface {
	Stale(stat fs.FileInfo) bool
}

type cacheCleanerFunc func(fs.FileInfo) bool

func (fn cacheCleanerFunc) Stale(info fs.FileInfo) bool {
	return fn(info)
}

type cacheDirectory struct {
	inner identity.Directory
	*fileCache
}

type fileCache struct {
	dir      string
	lifetime time.Duration
	disabled bool
	cleaner  CacheCleaner
	logger   *slog.Logger
}

func (fc *fileCache) Disable() { fc.disabled = true }

func (cd *cacheDirectory) Lookup(ctx context.Context, i syntax.AtIdentifier) (*identity.Identity, error) {
	var (
		err   error
		did   syntax.DID
		ident = new(identity.Identity)
	)
	if cd.disabled {
		return cd.inner.Lookup(ctx, i)
	}
	if i.IsHandle() {
		h, err := i.AsHandle()
		if err != nil {
			return nil, err
		}
		did, err = cd.cachedDID(h)
		if err != nil {
			goto fallback
		}
	} else if i.IsDID() {
		did, err = i.AsDID()
		if err != nil {
			return nil, err
		}
	}
	err = cd.cached(ident, cd.didPath(did), cd.lifetime, 0)
	if err != nil {
		goto fallback
	}
	return ident, nil
fallback:
	ident, err = cd.inner.Lookup(ctx, i)
	if err != nil {
		return nil, err
	}
	cd.stash(cd.handlePath(ident.Handle), ident)
	if i.IsHandle() {
		cd.stashDID(ident.Handle, ident.DID)
	}
	return ident, nil
}

func (cd *cacheDirectory) LookupHandle(ctx context.Context, h syntax.Handle) (*identity.Identity, error) {
	ident := new(identity.Identity)
	h = h.Normalize()
	did, err := cd.cachedDID(h)
	if err != nil {
		goto fallback
	}
	err = cd.cached(ident, cd.didPath(did), cd.lifetime, 0)
	if err != nil {
		goto fallback
	}
	return ident, nil
fallback:
	ident, err = cd.inner.LookupHandle(ctx, h)
	if err != nil {
		return nil, err
	}
	cd.stashDID(ident.Handle, ident.DID)
	cd.stash(cd.didPath(ident.DID), ident)
	return ident, nil
}

func (cd *cacheDirectory) LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error) {
	ident := new(identity.Identity)
	err := cd.cached(ident, cd.didPath(did), cd.lifetime, 0)
	if err != nil {
		ident, err = cd.inner.LookupDID(ctx, did)
		if err != nil {
			return nil, err
		}
		cd.stashDID(ident.Handle, ident.DID)
		cd.stash(cd.handlePath(ident.Handle), ident)
		return ident, nil
	}
	return ident, nil
}

func (cd *cacheDirectory) Purge(ctx context.Context, i syntax.AtIdentifier) error {
	defer func() {
		err := cd.inner.Purge(ctx, i) // inner purge no matter what
		if err != nil {
			slog.Warn("cache failed to call inner Purge", slog.Any("error", err))
		}
	}()
	if i.IsHandle() {
		h, err := i.AsHandle()
		if err != nil {
			return err
		}
		did, err := cd.cachedDID(h)
		if err == nil {
			err = cd.purgeDID(did)
			if err != nil {
				return err
			}
		}
		err = os.Remove(cd.handlePath(h.Normalize()))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if i.IsDID() {
		did, err := i.AsDID()
		if err != nil {
			return err
		}
		err = cd.purgeDID(did)
		if err != nil {
			return err
		}
	}
	return nil
}

func (cd *cacheDirectory) purgeDID(did syntax.DID) error {
	var err error
	err = os.Remove(cd.didPath(did))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	err = os.RemoveAll(filepath.Join(cd.dir, "records", did.String()))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (fc *fileCache) cachedDID(h syntax.Handle) (syntax.DID, error) {
	fp := fc.handlePath(h)
	_, err := fc.check(fp, fc.lifetime, DIDMaxLen)
	if err != nil {
		return "", err
	}
	f, err := os.Open(fp)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	if len(b) > DIDMaxLen {
		return "", ErrDIDTooLong
	}
	did, err := syntax.ParseDID(string(b))
	if err != nil {
		_ = os.Remove(fp)
		return "", err
	}
	return did, nil
}

func (fc *fileCache) stashDID(h syntax.Handle, did syntax.DID) {
	if fc.disabled {
		return
	}
	h = h.Normalize()
	fp := fc.handlePath(h)
	f, err := os.OpenFile(fp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		slog.Warn("failed to open did cache file", slog.Any("error", err))
		return
	}
	defer f.Close()
	_, err = f.WriteString(did.String())
	if err != nil {
		slog.Warn("failed to write did to cache file", slog.Any("error", err))
	}
}

func (fc *fileCache) cached(item any, path string, exp time.Duration, maxSize int64) error {
	if fc.disabled {
		return ErrNoCache
	}
	_, err := fc.check(path, exp, maxSize)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = json.NewDecoder(f).Decode(item); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func (fc *fileCache) stash(path string, item any) {
	if fc.disabled {
		return
	}
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0755)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		slog.Warn("failed to open cache file", slog.Any("error", err))
		return
	}
	err = json.NewEncoder(f).Encode(item)
	if err != nil {
		f.Close()
		_ = os.Remove(path)
		slog.Warn("failed to encode json", slog.Any("error", err))
		return
	}
	_ = f.Close()
}

func (fc *fileCache) check(path string, exp time.Duration, maxSize int64) (stat fs.FileInfo, err error) {
	stat, err = os.Stat(path)
	if err != nil {
		return
	}
	if maxSize != 0 && stat.Size() > maxSize {
		return stat, ErrFileTooBig
	}
	if time.Since(stat.ModTime()) > exp {
		_ = os.Remove(path)
		return stat, ErrCacheExpired
	}
	return stat, nil
}

func (fc *fileCache) path(name string, item interface{ String() string }) string {
	return filepath.Join(fc.dir, name, item.String())
}

func (fc *fileCache) handlePath(h syntax.Handle) string {
	return fc.path("handles", h)
}

func (fc *fileCache) didPath(did syntax.DID) string {
	return fc.path("dids", did)
}

func (fc *fileCache) start(ctx context.Context) {
	fc.clean()
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			fc.clean()
		case <-ctx.Done():
			return
		}
	}
}

func (fc *fileCache) clean() {
	err := filepath.Walk(fc.dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		i := fullFileInfo{path: path, FileInfo: info}
		stale := fc.cleaner.Stale(&i)
		if stale {
			fc.logger.Debug("cleaning cached file",
				"file", path)
			err = os.Remove(path)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		slog.Error("failed to clean cache", slog.Any("error", err))
	}
}

type fullFileInfo struct {
	path string
	fs.FileInfo
}

func (f *fullFileInfo) Name() string {
	return filepath.Join(f.path, f.FileInfo.Name())
}
