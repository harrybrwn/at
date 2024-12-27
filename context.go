package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/harrybrwn/xdg"

	"github.com/harrybrwn/at/atp"
)

var HttpClient = http.DefaultClient

type Context struct {
	ctx   context.Context
	cache *fileCache
	dir   identity.Directory

	verbose bool
	purge   bool
	limit   int
	cursor  string
	history bool
}

func (cctx *Context) WithCtx(ctx context.Context) *Context {
	cctx.ctx = ctx
	return cctx
}

func (cctx *Context) init(ctx context.Context) (err error) {
	cctx.ctx = ctx
	dir := atp.Resolver{HttpClient: HttpClient}
	dir.PlcURL, err = url.Parse("https://plc.directory")
	if err != nil {
		return err
	}
	dir.HandleResolver, err = atp.NewDefaultHandleResolver()
	if err != nil {
		return err
	}
	// cleaner := cacheCleanerFunc(func(fi fs.FileInfo) bool { return false })
	base := xdg.Cache("at")
	cleaner := cleaner{basedir: base}
	d := cache(&dir, &cleaner, base)
	go d.start(context.Background())
	cctx.cache = d.fileCache
	cctx.dir = d
	return nil
}

func newContext() *Context {
	return &Context{
		limit:  10,
		cursor: "none",
		ctx:    context.Background(),
	}
}

type cleaner struct {
	basedir string
}

func (c *cleaner) Stale(stat fs.FileInfo) bool {
	p, err := filepath.Rel(c.basedir, stat.Name())
	if err != nil {
		slog.Error("failed Rel", slog.Any("error", err))
		return false
	}
	parts := strings.Split(p, string(filepath.Separator))
	if len(parts) == 0 {
		return false
	}
	switch parts[0] {
	case "handles":
		return time.Since(stat.ModTime()) > 24*time.Hour
	case "records":
		return time.Since(stat.ModTime()) > 24*time.Hour
	case "repos":
		return time.Since(stat.ModTime()) > 10*time.Hour
	case "dids":
		return false
	}
	return false
}
