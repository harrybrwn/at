package actorstore

import (
	"context"
	"database/sql"
	"io"
	"iter"
	"log/slog"
	"slices"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/harrybrwn/db"
	"github.com/huandu/go-sqlbuilder"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/array"
	"github.com/harrybrwn/at/internal/parallel"
	"github.com/harrybrwn/at/internal/repo"
	"github.com/harrybrwn/at/internal/xiter"
	"github.com/harrybrwn/at/xrpc"
)

type BlobReader struct {
	datastore
	blobstore repo.BlobStore
}

func NewBlobReader(
	db db.DB,
	blobstore repo.BlobStore,
	did syntax.DID,
	key crypto.PrivateKeyExportable,
) *BlobReader {
	return &BlobReader{
		datastore: datastore{
			db:  db,
			did: did,
			key: key.(*crypto.PrivateKeyK256),
		},
		blobstore: blobstore,
	}
}

func (br *BlobReader) GetBlobMetadata(ctx context.Context, cid cid.Cid) (size int64, mimeType *string, err error) {
	query := "SELECT size, mimeType FROM blob WHERE cid = ? AND takedownRef IS NULL"
	rows, err := br.db.QueryContext(ctx, query, cid.String())
	if err != nil {
		return 0, nil, errors.WithStack(err)
	}
	var (
		sizeVal     int64
		mimeTypeVal sql.NullString
	)
	if err = db.ScanOne(rows, &sizeVal, &mimeTypeVal); err != nil {
		return 0, nil, xrpc.NewInvalidRequest("Blob not found").Wrap(errors.WithStack(err))
	}
	if mimeTypeVal.Valid {
		mimeType = &mimeTypeVal.String
	}
	return sizeVal, mimeType, nil
}

func (br *BlobReader) GetBlob(ctx context.Context, cid cid.Cid) (size int64, mimeType *string, stream io.ReadCloser, err error) {
	size, mimeType, err = br.GetBlobMetadata(ctx, cid)
	if err != nil {
		return 0, nil, nil, err
	}
	stream, err = br.blobstore.GetStream(ctx, cid)
	if err != nil {
		return 0, nil, nil, err
	}
	return size, mimeType, stream, nil
}

type ListBlobsOpts struct {
	Since, Cursor *string
}

func (br *BlobReader) ListBlobs(ctx context.Context, limit int, opts *ListBlobsOpts) ([]string, error) {
	var (
		query = "SELECT DISTINCT blobCid FROM record_blob"
		args  []any
		blobs []string
	)
	if opts == nil {
		opts = new(ListBlobsOpts)
	}
	if opts.Since != nil {
		query += " INNER JOIN record ON record.uri = record_blob.recordUri WHERE record.repoRev > ?"
		args = append(args, *opts.Since)
	}
	if opts.Cursor != nil {
		query += " AND blobCid > ?"
		args = append(args, *opts.Cursor)
	}
	query += " ORDER BY blobCid ASC LIMIT ?"
	args = append(args, limit)

	rows, err := br.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var blobCid string
		if err := rows.Scan(&blobCid); err != nil {
			return nil, err
		}
		blobs = append(blobs, blobCid)
	}
	return blobs, nil
}

type StatusAttr struct {
	Applied bool
	Ref     string
}

func (br *BlobReader) GetBlobTakedownStatus(ctx context.Context, cid string) (*StatusAttr, error) {
	query := "SELECT takedownRef FROM blob WHERE cid = ?"
	rows, err := br.db.QueryContext(ctx, query, cid)
	if err != nil {
		return nil, err
	}
	var takedownRef sql.NullString
	if err := db.ScanOne(rows, &takedownRef); err != nil {
		return nil, err
	}
	if takedownRef.Valid {
		return &StatusAttr{Applied: true, Ref: takedownRef.String}, nil
	}
	return &StatusAttr{Applied: false}, nil
}

func (br *BlobReader) GetRecordsForBlob(ctx context.Context, cid string) ([]string, error) {
	rows, err := br.db.QueryContext(ctx, `SELECT recordUri FROM record_blob WHERE blobCid = ?`, cid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []string
	for rows.Next() {
		var recordUri string
		if err := rows.Scan(&recordUri); err != nil {
			return nil, err
		}
		records = append(records, recordUri)
	}
	return records, nil
}

func (br *BlobReader) BlobCount(ctx context.Context) (int64, error) {
	query := "SELECT COUNT(*) FROM blob"
	rows, err := br.db.QueryContext(ctx, query)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	var count int64
	if err := db.ScanOne(rows, &count); err != nil {
		return 0, err
	}
	return count, nil
}

func (br *BlobReader) RecordBlobCount(ctx context.Context) (int64, error) {
	rows, err := br.db.QueryContext(ctx, "SELECT COUNT(DISTINCT blobCid) FROM record_blob")
	if err != nil {
		return 0, err
	}
	var count int64
	if err := db.ScanOne(rows, &count); err != nil {
		return 0, err
	}
	return count, nil
}

type MissingBlob struct {
	CID       string
	RecordURI string
}

func (br *BlobReader) ListMissingBlobs(ctx context.Context, cursor *string, limit int) ([]MissingBlob, error) {
	query := `
		SELECT rb.blobCid, rb.recordUri
		FROM record_blob rb
		WHERE NOT EXISTS (
			SELECT 1 FROM blob WHERE blob.cid = rb.blobCid
		) `
	var args []any

	if cursor != nil {
		query += " AND rb.blobCid > ?"
		args = append(args, *cursor)
	}
	query += " ORDER BY rb.blobCid ASC LIMIT ?"
	args = append(args, limit)
	rows, err := br.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()

	var missing []MissingBlob
	for rows.Next() {
		var blob MissingBlob
		if err := rows.Scan(&blob.CID, &blob.RecordURI); err != nil {
			return nil, errors.WithStack(err)
		}
		missing = append(missing, blob)
	}
	return missing, nil
}

type BlobTransactor struct{ *BlobReader }

func NewBlobTransactor(db db.DB, br *BlobReader) *BlobTransactor {
	return &BlobTransactor{BlobReader: NewBlobReader(db, br.blobstore, br.did, br.key)}
}

func (t *BlobTransactor) DeleteBlob(ctx context.Context, cid cid.Cid) error {
	// Delete the blob
	if err := t.blobstore.Delete(ctx, cid); err != nil {
		return err
	}

	query := "DELETE FROM blob WHERE cid = ?"
	_, err := t.db.ExecContext(ctx, query, cid.String())
	return err
}

func (t *BlobTransactor) QuarantineBlob(ctx context.Context, cid cid.Cid) error {
	return t.blobstore.Quarantine(ctx, cid)
}

func (t *BlobTransactor) UnQuarantineBlob(ctx context.Context, cid cid.Cid) error {
	return t.blobstore.Unquarantine(ctx, cid)
}

func (t *BlobTransactor) GetBlobDetails(ctx context.Context, cid string) (map[string]interface{}, error) {
	query := "SELECT size, mimeType, quarantined FROM blob WHERE cid = ?"
	rows, err := t.db.QueryContext(ctx, query, cid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		var size int64
		var mimeType sql.NullString
		var quarantined bool
		if err := rows.Scan(&size, &mimeType, &quarantined); err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"size":        size,
			"mimeType":    mimeType.String,
			"quarantined": quarantined,
		}, nil
	}
	return nil, errors.New("blob not found")
}

func (t *BlobTransactor) ListAllBlobs(ctx context.Context, limit int, offset int) ([]map[string]interface{}, error) {
	query := "SELECT cid, size, mimeType FROM blob ORDER BY created_at DESC LIMIT ? OFFSET ?"
	rows, err := t.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var blobs []map[string]interface{}
	for rows.Next() {
		var cid string
		var size int64
		var mimeType sql.NullString
		if err := rows.Scan(&cid, &size, &mimeType); err != nil {
			return nil, err
		}
		blobs = append(blobs, map[string]interface{}{
			"cid":      cid,
			"size":     size,
			"mimeType": mimeType.String,
		})
	}
	return blobs, nil
}

func (t *BlobTransactor) ProcessWriteBlobs(ctx context.Context, rev string, writes []repo.PreparedWrite) error {
	type jobdata struct {
		uri  syntax.ATURI
		blob *repo.PreparedBlobRef
	}
	data := make([]jobdata, 0, len(writes))
	for _, write := range writes {
		action := write.GetAction()
		if action == repo.WriteOpActionCreate || action == repo.WriteOpActionUpdate {
			blobs := write.GetBlobs()
			for i := range blobs {
				data = append(data, jobdata{
					uri:  write.GetURI(),
					blob: &blobs[i],
				})
			}
		}
	}
	err := t.deleteDereferencedBlobs(ctx, writes)
	if err != nil {
		return err
	}
	return parallel.Do(ctx,
		func(ctx context.Context) error {
			_, err := parallel.Map(ctx, data, func(ctx context.Context, jd jobdata) (any, error) {
				return nil, t.verifyBlobAndMakePermanent(jd.blob)
			})
			return err
		},
		func(ctx context.Context) error {
			_, err := parallel.Map(ctx, data, func(ctx context.Context, jd jobdata) (any, error) {
				return nil, t.associateBlob(jd.blob, jd.uri)
			})
			return err
		})
}

func (t *BlobTransactor) deleteDereferencedBlobs(ctx context.Context, writes []repo.PreparedWrite) error {
	uris := slices.Collect(array.FilterMap(array.Iter(writes), func(w *repo.PreparedWrite) (string, bool) {
		if w.PreparedDelete != nil {
			return w.PreparedDelete.URI.String(), true
		} else if w.PreparedUpdate != nil {
			return w.PreparedUpdate.URI.String(), true
		}
		return "", false
	}))
	query := `DELETE FROM record_blob WHERE recordUri IN (`
	args := make([]any, len(uris))
	for i, uri := range uris {
		args[i] = uri
		query += "?"
		if i < len(uris)-1 {
			query += ","
		}
	}
	query += ") RETURNING blobCid"
	rows, err := t.db.QueryContext(ctx, query, args...)
	if err != nil {
		return errors.WithStack(err)
	}
	deletedCids := slices.Collect(xiter.Map(func(c string) any { return c }, blobCids(rows)))
	if len(deletedCids) == 0 {
		return nil
	}
	qb := sqlbuilder.NewSelectBuilder()
	query, args = qb.Select("blobCid").From("record_blob").Where(qb.In("blobCid", deletedCids...)).Build()
	rows, err = t.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	duplicateCids := blobCids(rows)
	_ = duplicateCids
	return nil
}

type ScannableString string

func (s *ScannableString) Scan(scanner db.Scanner) error { return scanner.Scan(&s) }

func blobCids(r db.Rows) iter.Seq[string] {
	return func(yield func(string) bool) {
		defer r.Close()
		for r.Next() {
			var v string
			err := r.Scan(&v)
			if err != nil {
				slog.Error("failed to get blob cid", "error", err)
				return
			}
			if !yield(v) {
				return
			}
		}
	}
}

func (t *BlobTransactor) verifyBlobAndMakePermanent(blob *repo.PreparedBlobRef) error {
	return nil
}

func (t *BlobTransactor) associateBlob(blob *repo.PreparedBlobRef, uri syntax.ATURI) error {
	return nil
}
