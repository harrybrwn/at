package actorstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	database "github.com/harrybrwn/db"
	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/array"
	"github.com/harrybrwn/at/internal/parallel"
	"github.com/harrybrwn/at/internal/repo"
	"github.com/harrybrwn/at/internal/xiter"
)

type RepoBlock struct {
	cid     string
	repoRev string
	size    int
	content []byte
}

// SQLRepoReader implements the base read operations
type SQLRepoReader struct {
	*datastore
	cache *repo.BlockMap
	bs    blockstore.Blockstore
}

func NewSQLRepoReader(db database.DB, did syntax.DID, signingKey crypto.PrivateKeyExportable) *SQLRepoReader {
	return &SQLRepoReader{
		datastore: &datastore{
			db:  db,
			did: did,
			key: signingKey.(*crypto.PrivateKeyK256),
		},
		cache: repo.NewBlockMap(),
	}
}

func (s *SQLRepoReader) Close() error { return s.db.Close() }

func (s *SQLRepoReader) Tx(ctx context.Context, blob repo.BlobStore, fn func(ctx context.Context, tx *RepoTransactor) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	err = fn(ctx, NewRepoTransactor(tx, s.did, s.key, blob, nil, now))
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return errors.WithStack(tx.Commit())
}

func (s *SQLRepoReader) GetRoot(ctx context.Context) (cid.Cid, error) {
	root, err := s.GetRootDetailed(ctx)
	if err != nil {
		return cid.Cid{}, err
	}
	if root == nil {
		return cid.Cid{}, nil
	}
	return root.CID, nil
}

func (s *SQLRepoReader) GetRootDetailed(ctx context.Context) (*repo.RootInfo, error) {
	var (
		root repo.RootInfo
		cid  StoredCid
	)
	rows, err := s.db.QueryContext(ctx, "SELECT cid, rev FROM repo_root LIMIT 1")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = database.ScanOne(rows, &cid, &root.Rev)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to get root")
	}
	root.CID = cid.CID
	return &root, nil
}

func (s *SQLRepoReader) GetBytes(ctx context.Context, c cid.Cid) ([]byte, error) {
	// Check cache first
	if content, ok := s.cache.Get(c); ok {
		return content, nil
	}
	var content []byte
	rows, err := s.db.QueryContext(ctx, "SELECT content FROM repo_block WHERE cid = ?", c.String())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = database.ScanOne(rows, &content)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to get bytes")
	}
	s.cache.Set(c, content)
	return content, nil
}

func (s *SQLRepoReader) Has(ctx context.Context, cid cid.Cid) (bool, error) {
	content, err := s.GetBytes(ctx, cid)
	if err != nil {
		return false, err
	}
	return content != nil, nil
}

func (s *SQLRepoReader) GetBlocks(ctx context.Context, cids []cid.Cid) (*repo.BlockResult, error) {
	result := &repo.BlockResult{
		Blocks:  repo.NewBlockMap(),
		Missing: make([]cid.Cid, 0),
	}

	// Check memory cache first then fetch the missing CIDs
	cached, missing := s.cache.GetMany(cids)
	for _, entry := range cached.Entries() {
		result.Blocks.Set(entry.CID, entry.Bytes)
	}
	if len(missing) == 0 {
		return result, nil
	}

	// Fetch missing blocks from database
	query := `SELECT cid, content FROM repo_block WHERE cid IN (?` +
		strings.Repeat(",?", len(missing)-1) + ")"
	args := array.Map(array.Map(missing, array.ToString), array.ToAny)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query blocks")
	}
	defer rows.Close()

	foundCids := make(map[cid.Cid]struct{})
	for rows.Next() {
		var cid StoredCid
		var content []byte
		if err := rows.Scan(&cid, &content); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		result.Blocks.Set(cid.CID, content)
		s.cache.Set(cid.CID, content)
		foundCids[cid.CID] = struct{}{}
	}

	// Identify missing blocks
	for _, cid := range missing {
		if _, found := foundCids[cid]; !found {
			result.Missing = append(result.Missing, cid)
		}
	}
	return result, nil
}

func (s *SQLRepoReader) ListAllBlocks(ctx context.Context) (*repo.BlockMap, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT cid, content FROM repo_block`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var (
		m       = repo.NewBlockMap()
		cid     StoredCid
		content []byte
	)
	for rows.Next() {
		err = rows.Scan(&cid, &content)
		if err != nil {
			return nil, err
		}
		m.Set(cid.CID, content)
	}
	return m, nil
}

// SQLRepoTransactor extends SQLRepoReader with write operations
type SQLRepoTransactor struct {
	*SQLRepoReader
	now string
}

var _ repo.Storage = (*SQLRepoTransactor)(nil)

func NewSQLRepoTransactor(tx database.DB, did syntax.DID, key crypto.PrivateKeyExportable) *SQLRepoTransactor {
	rr := NewSQLRepoReader(tx, did, key)
	return &SQLRepoTransactor{
		SQLRepoReader: rr,
		now:           time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *SQLRepoTransactor) CacheRev(ctx context.Context, rev string) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT cid, content 
		FROM repo_block 
		WHERE repoRev = ? 
		LIMIT 15`, rev)
	if err != nil {
		return errors.Wrap(err, "failed to query blocks for rev")
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid     StoredCid
			content []byte
		)
		if err := rows.Scan(&cid, &content); err != nil {
			return errors.Wrap(err, "failed to scan row")
		}
		s.cache.Set(cid.CID, content)
	}
	return errors.WithStack(rows.Err())
}

func (s *SQLRepoTransactor) PutBlock(ctx context.Context, cid cid.Cid, block []byte, rev string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO repo_block (cid, repoRev, size, content)
		VALUES (?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		cid, rev, len(block), block)
	if err != nil {
		return errors.Wrap(err, "failed to put block")
	}
	s.cache.Set(cid, block)
	return nil
}

func (s *SQLRepoTransactor) PutMany(ctx context.Context, toPut *repo.BlockMap, rev string) error {
	blocks := xiter.Keys(xiter.Map2(func(cid cid.Cid, bytes []byte) (*RepoBlock, struct{}) {
		return &RepoBlock{
			cid:     cid.String(),
			repoRev: rev,
			size:    len(bytes),
			content: bytes,
		}, struct{}{}
	}, toPut.Iter()))
	errs := xiter.Map(func(blocks []*RepoBlock) error {
		query := "INSERT INTO repo_block (cid, repoRev, size, content) VALUES "
		query += strings.Repeat("(?,?,?,?)", len(blocks))
		args := make([]any, 0, 4*len(blocks))
		for i := 0; i < len(blocks); i++ {
			args = append(args, blocks[i].cid)
			args = append(args, blocks[i].repoRev)
			args = append(args, blocks[i].size)
			args = append(args, blocks[i].content)
		}
		_, err := s.db.ExecContext(ctx, query, args...)
		return errors.WithStack(err)
	}, xiter.Chunk(blocks, 50))
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// func (s *SQLRepoTransactor) DeleteMany(ctx context.Context, cids []syntax.CID) error {
func (s *SQLRepoTransactor) DeleteMany(ctx context.Context, cids *cid.Set) error {
	if cids.Len() == 0 {
		return nil
	}
	query := `DELETE FROM repo_block WHERE cid IN (?` +
		strings.Repeat(",?", cids.Len()-1) + ")"
	// args := array.Map(slices.Collect(cids.Iter()), array.ToAny)
	args := array.Map(slices.Collect(repo.IterCIDSet(cids)), array.ToAny)
	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete blocks")
	}
	for c := range repo.IterCIDSet(cids) {
		c, err := cid.Parse(c)
		if err != nil {
			return err
		}
		s.cache.Delete(c)
	}
	return nil
}

func (s *SQLRepoTransactor) ApplyCommit(ctx context.Context, commit *repo.CommitData, isCreate bool) error {
	return parallel.Do(
		ctx,
		func(ctx context.Context) error {
			return s.UpdateRoot(ctx, commit.CID, commit.Rev, isCreate)
		},
		func(ctx context.Context) error {
			return s.PutMany(ctx, commit.NewBlocks, commit.Rev)
		},
		func(ctx context.Context) error {
			return s.DeleteMany(ctx, commit.RemovedCIDs)
		},
	)
}

func (s *SQLRepoTransactor) UpdateRoot(ctx context.Context, cid cid.Cid, rev string, isCreate bool) error {
	if isCreate {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO repo_root (did, cid, rev, indexedAt)
			VALUES (?, ?, ?, ?)`,
			s.did, cid.String(), rev, s.now)
		if err != nil {
			return errors.Wrap(err, "failed to insert root")
		}
	} else {
		_, err := s.db.ExecContext(ctx, `
			UPDATE repo_root
			SET cid = ?, rev = ?, indexedAt = ?
			WHERE did = ?`,
			cid.String(), rev, s.now, s.did)
		if err != nil {
			return errors.Wrap(err, "failed to update root")
		}
	}
	return nil
}

type StoredCid struct {
	CID cid.Cid
}

func (sc *StoredCid) Scan(value any) (err error) {
	sc.CID, err = cid.Parse(value)
	if err != nil {
		return err
	}
	return nil
}

func (sc *StoredCid) Value() (driver.Value, error) {
	return sc.CID.String(), nil
}
