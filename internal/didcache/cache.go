package didcache

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/did"
	"github.com/harrybrwn/db"
	_ "github.com/mattn/go-sqlite3"
)

type Result struct {
	Doc       did.Document
	UpdatedAt time.Time
	DID       string
	Stale     bool
	Expired   bool
}

type DIDCache struct {
	db       *sql.DB
	staleTTL time.Duration
	maxTTL   time.Duration
}

func NewDIDCache(dbLocation string, staleTTL, maxTTL time.Duration) (*DIDCache, error) {
	db, err := sql.Open("sqlite3", dbLocation)
	if err != nil {
		return nil, err
	}
	cache := DIDCache{
		db:       db,
		staleTTL: staleTTL,
		maxTTL:   maxTTL,
	}
	if err := cache.initializeSchema(); err != nil {
		return nil, err
	}
	return &cache, nil
}

func (cache *DIDCache) initializeSchema() error {
	_, err := cache.db.Exec(`
CREATE TABLE IF NOT EXISTS "did_doc" (
	"did"       VARCHAR PRIMARY KEY,
	"doc"       TEXT    NOT NULL,
	"updatedAt" BIGINT  NOT NULL
);
CREATE TABLE IF NOT EXISTS "did_handle" (
	"handle"    VARCHAR PRIMARY KEY,
	"did"       VARCHAR NOT NULL,
	"updatedAt" BIGINT  NOT NULL
);`)
	if err != nil {
		return err
	}
	return nil
}

func (cache *DIDCache) CacheDoc(ctx context.Context, did string, doc *did.Document, prevResult *Result) error {
	docJSON, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	updatedAt := time.Now().UnixMilli()
	if prevResult != nil {
		query := `UPDATE did_doc SET doc = ?, updatedAt = ? WHERE did = ? AND updatedAt = ?`
		result, err := cache.db.ExecContext(ctx, query, string(docJSON), updatedAt, did, prevResult.UpdatedAt)
		if err != nil {
			return err
		}
		if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
			return errors.New("no rows updated")
		}
	} else {
		query := `INSERT INTO did_doc (did, doc, updatedAt) VALUES (?, ?, ?)
			ON CONFLICT(did) DO UPDATE SET doc = excluded.doc, updatedAt = excluded.updatedAt`
		_, err := cache.db.ExecContext(ctx, query, did, string(docJSON), updatedAt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (cache *DIDCache) CachedDoc(ctx context.Context, didstr string) (*Result, error) {
	var (
		docJSON   string
		updatedAt int64
	)
	query := `SELECT doc, updatedAt FROM did_doc WHERE did = ?`
	row := cache.db.QueryRowContext(ctx, query, didstr)
	if err := row.Scan(&docJSON, &updatedAt); err != nil {
		return nil, err
	}
	var doc did.Document
	if err := json.Unmarshal([]byte(docJSON), &doc); err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	stale := now > updatedAt+cache.staleTTL.Milliseconds()
	expired := now > updatedAt+cache.maxTTL.Milliseconds()
	return &Result{
		Doc:       doc,
		UpdatedAt: time.UnixMilli(updatedAt),
		DID:       didstr,
		Stale:     stale,
		Expired:   expired,
	}, nil
}

func (cache *DIDCache) StoreDID(ctx context.Context, handle, did string) error {
	query := `INSERT INTO did_handle (handle, did, updatedAt) VALUES (?, ?, ?)
		ON CONFLICT (handle) DO UPDATE SET did = excluded.did, updatedAt = excluded.updatedAt`
	updatedAt := time.Now().UnixMilli()
	_, err := cache.db.ExecContext(ctx, query, handle, did, updatedAt)
	return err
}

func (cache *DIDCache) GetDID(ctx context.Context, handle string) (*Result, error) {
	rows, err := cache.db.QueryContext(ctx,
		`SELECT did, updatedAt
		 FROM did_handle WHERE handle = ?`, handle)
	if err != nil {
		return nil, err
	}
	var (
		did       string
		updatedAt int64
	)
	if err = db.ScanOne(rows, &did, &updatedAt); err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	return &Result{
		DID:       did,
		UpdatedAt: time.UnixMilli(updatedAt),
		Stale:     now > updatedAt+cache.staleTTL.Milliseconds(),
		Expired:   now > updatedAt+cache.maxTTL.Milliseconds(),
	}, nil
}

type IdentityResult struct {
	Ident     identity.Identity
	UpdatedAt time.Time
	DID       string
	Stale     bool
	Expired   bool
}

func (cache *DIDCache) StoreIdentity(ctx context.Context, did string, ident *identity.Identity) error {
	b, err := json.Marshal(ident)
	if err != nil {
		return err
	}
	query := `INSERT INTO did_doc (did, doc, updatedAt) VALUES (?, ?, ?)
			ON CONFLICT(did)
			DO UPDATE SET doc = excluded.doc,
						  updatedAt = excluded.updatedAt`
	_, err = cache.db.ExecContext(ctx, query, did, string(b), time.Now().UnixMilli())
	return err
}

func (cache *DIDCache) GetIdentity(ctx context.Context, did string) (*IdentityResult, error) {
	var (
		rawDoc    string
		updatedAt int64
	)
	rows, err := cache.db.QueryContext(ctx, `SELECT doc, updatedAt FROM did_doc WHERE did = ?`, did)
	if err != nil {
		return nil, err
	}
	err = db.ScanOne(rows, &rawDoc, &updatedAt)
	if err != nil {
		return nil, err
	}
	var res IdentityResult
	err = json.Unmarshal([]byte(rawDoc), &res.Ident)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	res.Stale = now > updatedAt+cache.staleTTL.Milliseconds()
	res.Expired = now > updatedAt+cache.maxTTL.Milliseconds()
	res.UpdatedAt = time.UnixMilli(updatedAt)
	res.DID = did
	return &res, nil
}

func (cache *DIDCache) ClearEntry(ctx context.Context, did string) error {
	_, err := cache.db.ExecContext(ctx, `DELETE FROM did_doc WHERE did = ?`, did)
	return err
}

func (cache *DIDCache) ClearHandle(ctx context.Context, handle string) error {
	_, err := cache.db.ExecContext(ctx, `DELETE FROM did_handle WHERE handle = ?`, handle)
	return err
}

func (cache *DIDCache) Clear(ctx context.Context) error {
	_, e0 := cache.db.ExecContext(ctx, `DELETE FROM did_doc`)
	_, e1 := cache.db.ExecContext(ctx, `DELETE FROM did_handle`)
	if e0 != nil {
		return e0
	}
	if e1 != nil {
		return e1
	}
	return nil
}

func (cache *DIDCache) Close() error {
	return cache.db.Close()
}
