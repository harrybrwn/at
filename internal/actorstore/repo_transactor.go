package actorstore

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	indigorepo "github.com/bluesky-social/indigo/repo"
	"github.com/harrybrwn/db"
	"github.com/huandu/go-sqlbuilder"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/array"
	"github.com/harrybrwn/at/internal/blockstore"
	"github.com/harrybrwn/at/internal/parallel"
	"github.com/harrybrwn/at/internal/repo"
	"github.com/harrybrwn/at/xrpc"
)

// Custom errors
var (
	ErrInvalidRequest = errors.New("invalid request")
)

// PreparedCreate represents a create operation
type PreparedCreate struct {
	URI    syntax.ATURI
	CID    cid.Cid
	Record interface{}
}

type BackgroundQueue any

// RepoTransactor handles repository operations
type RepoTransactor struct {
	datastore
	backgroundQueue *BackgroundQueue
	blob            *BlobTransactor
	record          *RecordTransactor
	storage         *SQLRepoTransactor
	now             string
}

// NewRepoTransactor creates a new repository transactor
func NewRepoTransactor(
	db db.DB,
	did syntax.DID,
	signingKey crypto.PrivateKeyExportable,
	blobstore repo.BlobStore,
	backgroundQueue *BackgroundQueue,
	now string,
) *RepoTransactor {
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	ds := datastore{
		db:  db,
		did: did,
		key: signingKey.(*crypto.PrivateKeyK256),
	}
	return &RepoTransactor{
		datastore:       ds,
		blob:            NewBlobTransactor(db, NewBlobReader(db, blobstore, did, signingKey)),
		record:          newRecordTransactor(db, &ds),
		storage:         NewSQLRepoTransactor(db, did, signingKey),
		backgroundQueue: backgroundQueue,
		now:             now,
	}
}

// CreateRepo creates a new repository with the given writes
func (t *RepoTransactor) CreateRepo(ctx context.Context, writes []repo.PreparedCreate) (*repo.CommitData, error) {
	writeOps := make([]repo.RecordWriteOp, len(writes))
	preparedWrites := make([]repo.PreparedWrite, len(writes))
	for i, w := range writes {
		writeOps[i] = repo.RecordWriteOp{
			Action:     repo.WriteOpActionCreate,
			Collection: string(w.URI.Collection()),
			RecordKey:  string(w.URI.RecordKey()),
			Record:     w.Record,
		}
		preparedWrites[i] = repo.PreparedWrite{
			PreparedCreate: &writes[i],
		}
	}

	blk := blockstore.NewSQLStore(t.db, "")
	commit, err := repo.FormatInitCommit(ctx, blk, t.did.String(), t.key, writeOps)
	if err != nil {
		return nil, err
	}
	blk.SetRev(commit.Rev)
	err = parallel.Do(ctx,
		func(ctx context.Context) error {
			return t.storage.ApplyCommit(ctx, commit, true)
		},
		func(ctx context.Context) error {
			return t.indexWrites(ctx, preparedWrites, commit.Rev)
		},
		func(ctx context.Context) error {
			return t.blob.ProcessWriteBlobs(ctx, commit.Rev, preparedWrites)
		})
	if err != nil {
		return nil, err
	}
	return commit, nil
}

// ProcessWrites processes multiple write operations
func (t *RepoTransactor) ProcessWrites(ctx context.Context, writes []repo.PreparedWrite, swapCommitCID cid.Cid) (*repo.CommitData, error) {
	commit, err := t.FormatCommit(ctx, writes, swapCommitCID)
	if err != nil {
		return nil, err
	}
	err = parallel.Do(ctx,
		func(ctx context.Context) error {
			return t.storage.ApplyCommit(ctx, commit, true)
		},
		func(ctx context.Context) error {
			return t.indexWrites(ctx, writes, commit.Rev)
		},
		func(ctx context.Context) error {
			return t.blob.ProcessWriteBlobs(ctx, commit.Rev, writes)
		})
	if err != nil {
		return nil, err
	}
	return commit, nil
}

// FormatCommit formats a commit from the given writes
func (t *RepoTransactor) FormatCommit(
	ctx context.Context,
	writes []repo.PreparedWrite,
	swapCommit cid.Cid,
) (*repo.CommitData, error) {
	currRoot, err := t.storage.GetRootDetailed(ctx)
	if err != nil {
		return nil, err
	}
	if currRoot == nil {
		return nil, xrpc.NewInvalidRequest("No repo root found for %q", t.did)
	}
	if swapCommit.ByteLen() != 0 && currRoot.CID != swapCommit {
		return nil, repo.ErrBadCommitSwap
	}
	// Cache last commit
	if err := t.storage.CacheRev(ctx, currRoot.Rev); err != nil {
		return nil, err
	}

	newRecordCIDs := make([]cid.Cid, 0)
	delAndUpdateURIs := make([]syntax.ATURI, 0)
	for _, write := range writes {
		action := write.GetAction()
		if action != repo.WriteOpActionDelete {
			newRecordCIDs = append(newRecordCIDs, cid.MustParse(write.GetCID()))
		}
		if action != repo.WriteOpActionCreate {
			delAndUpdateURIs = append(delAndUpdateURIs, write.GetURI())
		}
		if write.GetSwapCID() == nil {
			continue
		}

		record, err := t.record.GetRecord(ctx, write.GetURI(), nil, true)
		if err != nil {
			return nil, err
		}
		var currRecord cid.Cid
		if record != nil {
			currRecord = record.CID
		}
		switch action {
		case repo.WriteOpActionCreate:
			if write.GetSwapCID() != nil {
				return nil, repo.ErrBadRecordSwap
			}
		case repo.WriteOpActionUpdate, repo.WriteOpActionDelete:
			if write.GetSwapCID() == nil {
				return nil, repo.ErrBadRecordSwap
			}
		}
		if (currRecord.ByteLen() != 0 || write.GetSwapCID() != nil) && !currRecord.Equals(*write.GetSwapCID()) {
			return nil, repo.ErrBadRecordSwap
		}
	}

	currRootCid, err := cid.Parse(currRoot.CID.String())
	if err != nil {
		return nil, err
	}
	blockstore := blockstore.NewSQLStore(t.db, "")
	repository, err := indigorepo.OpenRepo(ctx, blockstore, currRootCid)
	if err != nil {
		return nil, err
	}

	// repo, err := LoadRepo(ctx, t.storage, currRoot.CID)
	// if err != nil {
	// 	return nil, err
	// }

	writeOps := make([]repo.RecordWriteOp, len(writes))
	for i, w := range writes {
		uri := w.GetURI()
		writeOps[i] = repo.RecordWriteOp{
			Action:     w.GetAction(),
			Collection: string(uri.Collection()),
			RecordKey:  string(uri.RecordKey()),
			Record:     w.GetRecord(),
		}
	}

	// commit, err := repository.FormatCommit(ctx, writeOps, t.key)
	// if err != nil {
	// 	return nil, err
	// }
	commitCid, rev, err := repository.Commit(ctx, t.sign)
	if err != nil {
		return nil, err
	}
	blockstore.SetRev(rev)
	sc := repository.SignedCommit()
	commitData := repo.CommitData{
		CID:            commitCid,
		Rev:            rev,
		Prev:           cid.Undef,
		NewBlocks:      repo.NewBlockMap(),
		RelevantBlocks: repo.NewBlockMap(),
		// RemovedCIDs:    repo.NewCIDSet(nil),
		RemovedCIDs: cid.NewSet(),
	}
	if sc.Prev != nil {
		commitData.Prev = *sc.Prev
	}

	// Find duplicate record CIDs
	dupeCIDs, err := t.GetDuplicateRecordCIDs(ctx, commitData.RemovedCIDs, delAndUpdateURIs)
	if err != nil {
		return nil, err
	}

	for _, cid := range dupeCIDs {
		commitData.RemovedCIDs.Remove(cid)
	}

	// Handle missing blocks
	_, missing := commitData.RelevantBlocks.GetMany(newRecordCIDs)
	if len(missing) > 0 {
		missingBlocks, err := t.storage.GetBlocks(ctx, missing)
		if err != nil {
			return nil, err
		}
		commitData.RelevantBlocks.AddMap(missingBlocks.Blocks)
	}
	return &commitData, nil
}

func (t *RepoTransactor) sign(_ context.Context, did string, b []byte) ([]byte, error) {
	if did != t.did.String() {
		slog.Warn("repo signer got a mismatched did")
	}
	return t.key.HashAndSign(b)
}

// GetDuplicateRecordCIDs finds CIDs that are referenced by other records
func (t *RepoTransactor) GetDuplicateRecordCIDs(ctx context.Context, cids *cid.Set, touchedURIs []syntax.ATURI) ([]cid.Cid, error) {
	if len(touchedURIs) == 0 || cids.Len() == 0 {
		return nil, nil
	}
	cidStrs := slices.Collect(repo.IterCIDSet(cids))
	uriStrs := toString(touchedURIs)
	qb := sqlbuilder.SQLite.NewSelectBuilder()
	query, args := qb.Select("cid").
		From("reocrd").
		Where(
			qb.In("cid", array.Map(cidStrs, array.ToAny)...),
			qb.Not(qb.In("uri", array.Map(uriStrs, array.ToAny)...)),
		).
		Build()
	rows, err := t.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()
	var result []cid.Cid
	for rows.Next() {
		var cidStr StoredCid
		if err := rows.Scan(&cidStr); err != nil {
			return nil, err
		}
		result = append(result, cidStr.CID)
	}
	return result, nil
}

func (t *RepoTransactor) indexWrites(ctx context.Context, writes []repo.PreparedWrite, rev string) error {
	now := time.Now()
	writePtrs := slices.Collect(array.IterRef(writes))
	_, err := parallel.Map(ctx, writePtrs, func(ctx context.Context, w *repo.PreparedWrite) (any, error) {
		var (
			err    error
			action = w.GetAction()
			uri    = w.GetURI()
		)
		switch action {
		case repo.WriteOpActionCreate, repo.WriteOpActionUpdate:
			err = t.record.IndexRecord(ctx, uri, w.GetCID(), w.GetRecord(), action, rev, now)
		case repo.WriteOpActionDelete:
			err = t.record.DeleteRecord(ctx, uri)
		}
		return nil, err
	})
	return errors.WithStack(err)
}

func toString[Slice ~[]T, T fmt.Stringer](s Slice) []string {
	return array.Map(s, func(e T) string { return e.String() })
}
