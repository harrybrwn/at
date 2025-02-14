package main

import (
	"context"
	"database/sql"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/did"
	"github.com/harrybrwn/xdg"

	"github.com/harrybrwn/at/atp"
	"github.com/harrybrwn/at/internal/didcache"
	"github.com/harrybrwn/at/internal/httpcache"
	"github.com/harrybrwn/at/internal/sqlite"
)

var HttpClient = &http.Client{
	Transport: http.DefaultTransport,
	// Transport: &httpcache.Debugger{
	// 	RoundTripper: http.DefaultTransport,
	// },
}

type Context struct {
	ctx      context.Context
	dir      identity.Directory
	resolver did.Resolver
	logger   *slog.Logger

	verbose bool
	purge   bool
	noCache bool
	limit   int
	cursor  string
	history bool
	diddoc  bool

	cacheDB *sql.DB
	client  *http.Client

	// runtime metadata
	start time.Time
}

func (cctx *Context) cleanup() error {
	err := cctx.cacheDB.Close()
	if err != nil {
		return err
	}
	cctx.logger.Debug("end", "time", time.Since(cctx.start).String())
	return nil
}

func newContext() *Context {
	return &Context{
		limit:  50,
		cursor: "none",
		ctx:    context.Background(),
		client: HttpClient,
		start:  time.Now(),
		logger: slog.Default(),
	}
}

func (cctx *Context) WithCtx(ctx context.Context) *Context {
	cctx.ctx = ctx
	return cctx
}

func (cctx *Context) init(ctx context.Context) (err error) {
	cctx.ctx = ctx
	base := xdg.Cache("at")
	cachepath := filepath.Join(base, "cache.sqlite")
	cacheinit := !exists(cachepath)
	cctx.cacheDB, err = sqlite.Open(
		cachepath,
		&sqlite.Config{JournalMode: "WAL"},
	)
	if err != nil {
		return err
	}
	didcacher := didcache.New(
		cctx.cacheDB, time.Hour*12, time.Hour*24*7,
	)
	httpcacher := httpcache.New(
		cctx.cacheDB,
		HttpClient.Transport,
		time.Hour*24*7,
	)
	if cacheinit {
		err = didcacher.InitializeSchema()
		if err != nil {
			return err
		}
		err = httpcacher.Migrate(ctx)
		if err != nil {
			return err
		}
	}
	if !cctx.noCache {
		cctx.client = &http.Client{Transport: httpcacher}
		HttpClient.Transport = httpcacher
	}
	if cctx.purge {
		err = didcacher.Clear(ctx)
		if err != nil {
			return err
		}
		err = httpcacher.Purge(ctx)
		if err != nil {
			return err
		}
	}

	resolver := atp.Resolver{HttpClient: HttpClient}
	resolver.PlcURL, err = url.Parse("https://plc.directory")
	if err != nil {
		return err
	}
	resolver.HandleResolver, err = atp.NewDefaultHandleResolver()
	if err != nil {
		return err
	}

	if cctx.noCache {
		cctx.dir = &resolver
		cctx.resolver = &resolver
	} else {
		dir := didcache.NewDirectory(&resolver, &resolver, &resolver, didcacher)
		cctx.dir = dir
		cctx.resolver = didcache.NewDIDResolver(&resolver, didcacher)
	}
	return nil
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

func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
