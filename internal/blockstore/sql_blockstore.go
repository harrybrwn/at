package blockstore

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/harrybrwn/db"
	"github.com/ipfs/boxo/blockstore"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/pkg/errors"
)

var (
	_ blockstore.Blockstore   = (*SQLBlockstore)(nil)
	_ cbornode.IpldBlockstore = (*SQLBlockstore)(nil)
	_ datastore.Batching      = (*batching)(nil)
)

type SQLBlockstore struct {
	db         db.DB
	prefix     cid.Prefix
	hashOnRead bool
	rev        string
}

func NewSQLStore(db db.DB, rev string) *SQLBlockstore {
	return &SQLBlockstore{
		db:  db,
		rev: rev,
		prefix: cid.NewPrefixV1(
			uint64(multicodec.DagCbor),
			multihash.SHA2_256,
		),
		hashOnRead: true,
	}
}

func (b *SQLBlockstore) Migrate(ctx context.Context) error {
	_, err := b.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS "repo_block" (
  "cid" varchar primary key,
  "repoRev" varchar not null,
  "size" integer not null,
  "content" blob not null
);

CREATE INDEX IF NOT EXISTS "repo_block_repo_rev_idx" on "repo_block" ("repoRev", "cid");
`)
	return err
}

func NewBatching(db db.DB, rev string) *batching {
	return &batching{db: db, rev: rev}
}

func (b *SQLBlockstore) SetRev(rev string) { b.rev = rev }

func (b *SQLBlockstore) DeleteBlock(ctx context.Context, cid cid.Cid) error {
	_, err := b.db.ExecContext(ctx, `DELETE FROM repo_block WHERE cid = ?`, cid.String())
	return errors.WithStack(err)
}

func (b *SQLBlockstore) Has(ctx context.Context, cid cid.Cid) (bool, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT 1 FROM repo_block WHERE cid = ?`, cid.String())
	if err != nil {
		return false, errors.WithStack(err)
	}
	var c int
	err = db.ScanOne(rows, &c)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}
	return c == 1, nil
}

func (b *SQLBlockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT content FROM repo_block WHERE cid = ?`, c.String())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var content []byte
	err = db.ScanOne(rows, &content)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if b.hashOnRead {
		rehashedCid, err := b.prefix.Sum(content)
		if err != nil {
			return nil, err
		}
		return blocks.NewBlockWithCid(content, rehashedCid)
	} else {
		return blocks.NewBlockWithCid(content, c)
	}
}

func (b *SQLBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT size FROM repo_block WHERE cid = ?`, c.String())
	if err != nil {
		return 0, errors.WithStack(err)
	}
	var size int
	err = db.ScanOne(rows, &size)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return size, nil
}

func (b *SQLBlockstore) Put(ctx context.Context, blk blocks.Block) error {
	cid := blk.Cid()
	content := blk.RawData()
	repoRev := b.rev
	if rever, ok := blk.(interface{ Rev() string }); ok && len(repoRev) == 0 {
		repoRev = rever.Rev()
	}
	_, err := b.db.ExecContext(
		ctx,
		`INSERT INTO repo_block (cid, repoRev, size, content) VALUES (?, ?, ?, ?)`,
		cid.String(), repoRev, len(content), content,
	)
	return err
}

func (b *SQLBlockstore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	args := make([]any, 0, len(blocks)*4)
	values := ""
	for i, blk := range blocks {
		cid := blk.Cid()
		content := blk.RawData()
		repoRev := b.rev
		if rever, ok := blk.(interface{ Rev() string }); ok && len(repoRev) == 0 {
			repoRev = rever.Rev()
		}
		args = append(args, cid, repoRev, len(content), content)
		values += "(?, ?, ?, ?)"
		if i < len(blocks)-1 {
			values += ","
		}
	}
	_, err := b.db.ExecContext(
		ctx,
		`INSERT INTO repo_block (cid, repoRev, size, content) VALUES `+values,
		args...,
	)
	return errors.WithStack(err)
}

func (b *SQLBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT cid FROM repo_block`)
	if err != nil {
		return nil, err
	}
	ch := make(chan cid.Cid)
	go func() {
		defer close(ch)
		defer rows.Close()
		for rows.Next() {
			var b []byte
			err = rows.Scan(&b)
			if err != nil {
				slog.Warn("AllKeysChan: failed to pull cid from repo_block", "error", err)
				continue
			}
			_, c, err := cid.CidFromBytes(b)
			if err != nil {
				slog.Warn("AllKeysChan: failed to parse cid from bytes", "error", err)
				continue
			}
			select {
			case <-ctx.Done():
				return
			case ch <- c:
			}
		}
	}()
	return ch, nil
}

func (b *SQLBlockstore) view(c cid.Cid, fn func([]byte) error) error {
	rows, err := b.db.QueryContext(context.Background(), `SELECT content WHERE cid = ?`, c.String())
	if err != nil {
		return err
	}
	var content []byte
	if err = db.ScanOne(rows, &content); err != nil {
		return err
	}
	return fn(content)
}

func (b *SQLBlockstore) HashOnRead(enabled bool) {
	b.hashOnRead = enabled
}

type batching struct {
	db  db.DB
	rev string
}

func (b *batching) SetRev(rev string) { b.rev = rev }

func (b *batching) Close() error { return b.db.Close() }

func (b *batching) Delete(ctx context.Context, key datastore.Key) error {
	_, err := b.db.ExecContext(ctx, `DELETE FROM repo_block WHERE cid = ?`, key.String())
	return errors.WithStack(err)
}

func (b *batching) Get(ctx context.Context, key datastore.Key) ([]byte, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT content FROM repo_block WHERE cid = ?`, key.String())
	if err != nil {
		return nil, err
	}
	var content []byte
	if err = db.ScanOne(rows, &content); err != nil {
		return nil, errors.WithStack(err)
	}
	return content, nil
}

func (b *batching) Has(ctx context.Context, key datastore.Key) (bool, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT 1 FROM repo_block WHERE cid = ?`, key.String())
	if err != nil {
		return false, errors.WithStack(err)
	}
	var c int
	err = db.ScanOne(rows, &c)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}
	return c == 1, nil
}

func (b *batching) GetSize(ctx context.Context, key datastore.Key) (int, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT size FROM repo_block WHERE cid = ?`, key.String())
	if err != nil {
		return 0, errors.WithStack(err)
	}
	var size int
	err = db.ScanOne(rows, &size)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return size, nil
}

func (b *batching) Put(ctx context.Context, key datastore.Key, value []byte) error {
	_, err := b.db.ExecContext(
		ctx,
		`INSERT INTO repo_block (cid, repoRev, size, content) VALUES (?, ?, ?, ?)`,
		key.String(), b.rev, len(value), value,
	)
	return err
}

func (b *batching) Query(ctx context.Context, q query.Query) (query.Results, error) {
	if q.KeysOnly {
		keys, err := b.queryKeys(ctx, q)
		if err != nil {
			return nil, err
		}
		return query.ResultsWithEntries(q, keys), nil
	}
	rows, err := b.db.QueryContext(ctx, `SELECT cid, content FROM repo_block`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]query.Entry, 0)
outer:
	for rows.Next() {
		var (
			cid     string
			content []byte
		)
		err = rows.Scan(&cid, content)
		if err != nil {
			return nil, err
		}
		e := query.Entry{
			Key:   cid,
			Value: content,
			Size:  len(content),
		}
		for _, f := range q.Filters {
			if !f.Filter(e) {
				continue outer
			}
		}
		entries = append(entries, e)
	}
	return query.ResultsWithEntries(q, entries), nil
}

func (b *batching) queryKeys(ctx context.Context, _ query.Query) ([]query.Entry, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT cid, content FROM repo_block`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]query.Entry, 0)
	for rows.Next() {
		var k string
		if err = rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, query.Entry{Key: k})
	}
	return keys, nil
}

func (b *batching) Batch(ctx context.Context) (datastore.Batch, error) {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &repoBlockBatch{batching{db: tx}, tx}, nil
}

func (b *batching) Sync(ctx context.Context, prefix datastore.Key) error {
	return nil
}

func (b *batching) View(c cid.Cid, fn func([]byte) error) error {
	rows, err := b.db.QueryContext(context.Background(), `SELECT content WHERE cid = ?`, c.String())
	if err != nil {
		return err
	}
	var content []byte
	if err = db.ScanOne(rows, &content); err != nil {
		return err
	}
	return fn(content)
}

type repoBlockBatch struct {
	batching
	tx db.Tx
}

func (rb *repoBlockBatch) Commit(context.Context) error {
	return rb.tx.Commit()
}

var _ datastore.Batch = (*repoBlockBatch)(nil)
