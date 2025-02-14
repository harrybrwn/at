package httpcache

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/gob"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// Cache is an [http.RoundTripper] that aggressively caches http responses.
type Cache struct {
	RoundTripper http.RoundTripper
	TTL          time.Duration
	Logger       *slog.Logger
	db           *sql.DB
}

func New(db *sql.DB, rt http.RoundTripper, ttl time.Duration) *Cache {
	return &Cache{
		RoundTripper: rt,
		TTL:          ttl,
		Logger:       slog.Default(),
		db:           db,
	}
}

func (c *Cache) Migrate(ctx context.Context) error {
	_, err := c.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS "http_cache" (
	"key"       VARCHAR NOT NULL,
	"blob"      TEXT    NOT NULL,
	"updatedAt" BIGINT  NOT NULL
);
CREATE INDEX IF NOT EXISTS "http_cache_key_idx" ON "http_cache" ("key");`)
	return err
}

func Purge(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM "http_cache"`)
	return err
}

func (c *Cache) Purge(ctx context.Context) error { return Purge(ctx, c.db) }

func (c *Cache) RoundTrip(r *http.Request) (*http.Response, error) {
	var (
		err  error
		ctx  = r.Context()
		hash = sha256.New()
	)
	for _, b := range [][]byte{
		{byte(r.ProtoMajor), byte(r.ProtoMinor)},
		[]byte(r.Method),
		[]byte(r.URL.String()),
		[]byte(r.Header.Get("Content-Type")),
		[]byte(r.Header.Get("Content-Length")),
	} {
		_, err = hash.Write(b)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	key := hex.EncodeToString(hash.Sum(nil))
	cached, updatedAt, err := c.get(ctx, key)
	if err == nil && time.Now().UnixMilli() < updatedAt+c.TTL.Milliseconds() {
		length := int64(len(cached.Body))
		c.Logger.Debug("found response in cache",
			"method", r.Method,
			"url", r.URL.String(),
			"length", length,
			"key", key)
		return &http.Response{
			Request:       r,
			Proto:         cached.Proto,
			ProtoMajor:    cached.ProtoMajor,
			ProtoMinor:    cached.ProtoMinor,
			StatusCode:    cached.Status,
			Status:        http.StatusText(cached.Status),
			Header:        cached.Header,
			Body:          io.NopCloser(bytes.NewBuffer(cached.Body)),
			ContentLength: length,
		}, nil
	}

	response, err := c.RoundTripper.RoundTrip(r)
	if err != nil {
		return response, errors.WithStack(err)
	}
	var cachedbody bytes.Buffer
	err = captureResponseBody(response, &cachedbody)
	if err != nil {
		return response, errors.WithStack(err)
	}
	status := response.StatusCode
	if status < 300 || status == http.StatusPermanentRedirect || status == http.StatusMovedPermanently {
		err = c.put(ctx, key, response, &cachedbody)
		if err != nil {
			return response, errors.WithStack(err)
		}
		c.Logger.Debug("response stored in cache",
			"method", r.Method,
			"url", r.URL.String(),
			"length", response.ContentLength,
			"key", key)
	}
	return response, nil
}

func (c *Cache) get(ctx context.Context, key string) (*Result, int64, error) {
	var (
		res       Result
		updatedAt int64
		blob      []byte
	)
	row := c.db.QueryRowContext(ctx, `SELECT blob, updatedAt FROM "http_cache" WHERE key = ?`, key)
	err := row.Scan(&blob, &updatedAt)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}
	err = gob.NewDecoder(bytes.NewBuffer(blob)).Decode(&res)
	if err != nil {
		return nil, updatedAt, errors.WithStack(err)
	}
	return &res, updatedAt, nil
}

func (c *Cache) put(ctx context.Context, key string, res *http.Response, body *bytes.Buffer) error {
	data := Result{
		Proto:      res.Proto,
		ProtoMajor: res.ProtoMajor,
		ProtoMinor: res.ProtoMinor,
		Status:     res.StatusCode,
		Header:     res.Header,
		Body:       body.Bytes(),
	}
	var blob bytes.Buffer
	err := gob.NewEncoder(&blob).Encode(&data)
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = c.db.ExecContext(
		ctx,
		`INSERT INTO "http_cache" ("key", "blob", "updatedAt")
		VALUES (?, ?, ?)`,
		key,
		blob.Bytes(),
		time.Now().UnixMilli(),
	)
	return errors.WithStack(err)
}

func captureResponseBody(response *http.Response, cached *bytes.Buffer) error {
	var newbody bytes.Buffer
	_, err := io.Copy(
		cached,
		io.TeeReader(response.Body, &newbody),
	)
	if err != nil {
		response.Body.Close()
		return errors.WithStack(err)
	}
	if err = response.Body.Close(); err != nil {
		return errors.WithStack(err)
	}
	response.Body = io.NopCloser(&newbody)
	return nil
}

type Result struct {
	Proto      string
	ProtoMajor int
	ProtoMinor int
	Status     int
	Header     http.Header
	Body       []byte
}

func init() {
	gob.Register(&Result{})
}
