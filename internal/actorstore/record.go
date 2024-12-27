package actorstore

import (
	"context"
	"database/sql"
	"fmt"
	"iter"
	"reflect"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/fxamacker/cbor/v2"
	"github.com/harrybrwn/db"
	"github.com/huandu/go-sqlbuilder"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"

	comatp "github.com/harrybrwn/at/api/com/atproto"
	"github.com/harrybrwn/at/internal/cbor/dagcbor"
	"github.com/harrybrwn/at/internal/parallel"
	"github.com/harrybrwn/at/internal/repo"
)

type RecordReader datastore

type Record struct {
	URI         syntax.ATURI
	CID         cid.Cid
	Value       any
	Collection  string
	Rkey        string
	RepoRev     string
	TakedownRef sql.NullString
}

func (rr *RecordReader) Close() error { return rr.db.Close() }

func (rr *RecordReader) GetRecord(ctx context.Context, uri syntax.ATURI, cid *cid.Cid, includeSoftDeleted bool) (*Record, error) {
	query := `
        SELECT
            r.cid,
			r.uri,
            rb.content,
			r.collection,
			r.rkey,
			r.repoRev,
			r.takedownRef
        FROM record r
        INNER JOIN repo_block rb ON r.cid = rb.cid
        WHERE r.uri = ?`
	args := []any{uri.String()}
	if cid != nil {
		query += " AND r.cid = ?"
		args = append(args, cid.String())
	}
	if !includeSoftDeleted {
		query += ` AND r.takedownRef is null`
	}
	rows, err := rr.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	var (
		res       Record
		content   []byte
		storedcid StoredCid
	)
	err = db.ScanOne(
		rows,
		&storedcid,
		&res.URI,
		&content,
		&res.Collection,
		&res.Rkey,
		&res.RepoRev,
		&res.TakedownRef,
	)
	if err != nil {
		return nil, err
	}
	res.CID = storedcid.CID
	value := make(map[string]any)
	err = dagcbor.Unmarshal(content, &value)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	res.Value = value
	return &res, err
}

func (rr *RecordReader) ListCollections(ctx context.Context) (iter.Seq2[string, error], error) {
	rows, err := rr.db.QueryContext(ctx, `SELECT DISTINCT collection FROM record`)
	if err != nil {
		return nil, err
	}
	return basicIterScan[string](rows), nil
}

func (rr *RecordReader) ListForCollection(
	ctx context.Context,
	req *comatp.RepoListRecordsParams,
) (*comatp.RepoListRecordsResponse, error) {
	const baseQuery = `
	       SELECT
	           record.uri,
	           record.cid,
			repo_block.content
	       FROM record
	       INNER JOIN repo_block ON repo_block.cid = record.cid
	       WHERE record.collection = ? and record.takedownRef is null`
	query := baseQuery
	args := []any{req.Collection.String()}
	if req.Cursor != nil {
		args = append(args, *req.Cursor)
		if req.Reverse != nil && *req.Reverse {
			query += " AND record.rkey > ?"
		} else {
			query += " AND record.rkey < ?"
		}
	} else {
		if req.RkeyStart != nil {
			query += " AND record.rkey > ?"
			args = append(args, *req.RkeyStart)
		}
		if req.RkeyEnd != nil {
			query += " AND record.rkey < ?"
			args = append(args, *req.RkeyEnd)
		}
	}
	query += " ORDER BY record.rkey "
	if req.Reverse != nil && *req.Reverse {
		query += "ASC"
	} else {
		query += "DESC"
	}
	if req.Limit != nil {
		query += fmt.Sprintf(" LIMIT %d", *req.Limit)
	}

	// qb := sqlbuilder.SQLite.NewSelectBuilder()
	// qb.Select("record.uri", "record.cid", "repo_block.content").
	// 	From("record").
	// 	JoinWithOption(
	// 		sqlbuilder.InnerJoin,
	// 		"repo_block",
	// 		"record.cid = repo_block.cid",
	// 	).
	// 	Where(
	// 		qb.Equal("record.collection", req.Collection),
	// 		"record.takedownRef is null",
	// 	).
	// 	OrderBy("record.rkey")
	// if req.Cursor != nil {
	// 	if req.Reverse != nil && *req.Reverse {
	// 		qb.Where(qb.GreaterThan("record.rkey", *req.Cursor))
	// 	} else {
	// 		qb.Where(qb.LessThan("record.rkey", *req.Cursor))
	// 	}
	// } else {
	// 	if req.RkeyStart != nil {
	// 		qb.Where(qb.GreaterThan("record.rkey", *req.RkeyStart))
	// 	}
	// 	if req.RkeyEnd != nil {
	// 		qb.Where(qb.LessThan("record.rkey", *req.RkeyEnd))
	// 	}
	// }
	// if req.Limit != nil {
	// 	qb.Limit(int(*req.Limit))
	// }
	// if req.Reverse != nil && *req.Reverse {
	// 	qb.Asc()
	// } else {
	// 	qb.Desc()
	// }
	// query, args := qb.Build()

	rows, err := rr.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]comatp.RepoListRecordsRecord, 0)
	for rows.Next() {
		var (
			r       comatp.RepoListRecordsRecord
			content []byte
		)
		err := rows.Scan(&r.URI, &r.CID, &content)
		if err != nil {
			return nil, err
		}
		value := make(map[string]any)
		err = cbor.Unmarshal(content, &value)
		if err != nil {
			return nil, err
		}
		r.Value, err = strMapProcess(value)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return &comatp.RepoListRecordsResponse{
		Records: records,
	}, nil
}

type RecordBacklink struct {
	URI         string
	Collection  string
	CID         string
	RKey        string
	RepoRev     string
	IndexedAt   string
	TakedownRef string
}

func (rr *RecordReader) GetRecordBacklinks(ctx context.Context, collection syntax.NSID, path, linkTo string) ([]RecordBacklink, error) {
	query := `
        SELECT
			record.uri,
			record.collection,
			record.cid,
			record.rkey,
			record.repoRev,
			record.indexedAt,
			record.takedownRef
        FROM record 
        INNER JOIN backlink ON backlink.uri = record.uri 
        WHERE backlink.path = ? 
        AND backlink.linkTo = ? 
        AND record.collection = ?`
	rows, err := rr.db.QueryContext(ctx, query, path, linkTo, collection.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []RecordBacklink
	for rows.Next() {
		var record RecordBacklink
		if err := rows.Scan(
			&record.URI,
			&record.Collection,
			&record.CID,
			&record.RKey,
			&record.RepoRev,
			&record.IndexedAt,
			&record.TakedownRef,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (rr *RecordReader) GetBacklinkConflicts(ctx context.Context, uri syntax.ATURI, record LinkableRecord) ([]syntax.ATURI, error) {
	recordBacklinks := getBacklinks(uri, record)
	conflicts := make([][]RecordBacklink, len(recordBacklinks))
	// Process each backlink concurrently
	errChan := make(chan error, len(recordBacklinks))
	var wg sync.WaitGroup
	wg.Add(len(recordBacklinks))
	go func() {
		wg.Wait()
		close(errChan)
	}()
	for i, backlink := range recordBacklinks {
		go func(idx int, bl Backlink) {
			defer wg.Done()
			records, err := rr.GetRecordBacklinks(
				ctx,
				uri.Collection(),
				bl.Path,
				bl.LinkTo,
			)
			if err != nil {
				errChan <- err
				return
			}
			conflicts[idx] = records
		}(i, backlink)
	}

	// Check for errors from goroutines
	for i := 0; i < len(recordBacklinks); i++ {
		if err := <-errChan; err != nil {
			return nil, err
		}
	}

	// Flatten results and convert to ATURIs
	var result []syntax.ATURI
	for _, recordList := range conflicts {
		for _, record := range recordList {
			uri, err := syntax.ParseATURI(record.URI)
			if err != nil {
				return nil, err
			}
			result = append(result, uri)
		}
	}
	return result, nil
}

func (rr *RecordReader) Tx(ctx context.Context, fn func(ctx context.Context, tx *RecordTransactor) error) error {
	tx, err := rr.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	err = fn(ctx, newRecordTransactor(tx, (*datastore)(rr)))
	if err != nil {
		_ = tx.Rollback()
		return errors.WithStack(err)
	}
	return errors.WithStack(tx.Commit())
}

type RecordTransactor struct {
	*RecordReader
}

func newRecordTransactor(tx db.DB, ds *datastore) *RecordTransactor {
	return &RecordTransactor{
		RecordReader: (*RecordReader)(&datastore{
			db:  tx,
			did: ds.did,
			key: ds.key,
		}),
	}
}

func (rt *RecordTransactor) IndexRecord(
	ctx context.Context,
	uri syntax.ATURI,
	cid cid.Cid,
	record any,
	action repo.WriteOpAction,
	repoRev string,
	timestamp time.Time,
) error {
	if !uri.Authority().IsDID() {
		return errors.New("expected indexed uri to use a did")
	} else if len(uri.Collection()) < 1 {
		return errors.New("indexed uri must contain a collection")
	} else if len(uri.RecordKey()) < 1 {
		return errors.New("indexed uri must contain an rkey")
	}
	_, err := rt.db.ExecContext(ctx,
		`INSERT INTO record (
			uri, cid,
			collection,
			rkey,
			repoRev,
			indexedAt
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (uri) DO UPDATE SET
			cid = excluded.cid,
			repoRev = excluded.repoRev,
			indexedAt = excluded.indexedAt`,
		uri.String(),
		cid,
		uri.Collection().String(),
		uri.RecordKey().String(),
		repoRev,
		timestamp.Format(time.RFC3339),
	)
	if err != nil {
		return errors.WithStack(err)
	}
	if record == nil {
		return nil
	}
	if action == repo.WriteOpActionUpdate {
		return rt.RemoveBacklinksByURI(ctx, uri)
	}
	linkableRecord, isLinkable := record.(LinkableRecord)
	mapRecord, isMap := record.(map[string]any)
	if !isLinkable {
		if !isMap {
			return nil
		} else {
			linkableRecord = RepoRecord(mapRecord)
		}
	}
	backlinks := getBacklinks(uri, linkableRecord)
	return rt.AddBacklinks(ctx, backlinks)
}

func (rt *RecordTransactor) RemoveBacklinksByURI(ctx context.Context, uri syntax.ATURI) error {
	_, err := rt.db.ExecContext(ctx, `DELETE FROM backlink WHERE uri = ?`, uri.String())
	return errors.WithStack(err)
}

func (rt *RecordTransactor) AddBacklinks(ctx context.Context, backlinks []Backlink) error {
	qb := sqlbuilder.InsertInto("backlink").
		Cols("uri", "path", "linkTo")
	for _, bl := range backlinks {
		qb.Values(bl.URI, bl.Path, bl.LinkTo)
	}
	query, args := qb.Build()
	_, err := rt.db.ExecContext(ctx, query, args...)
	return errors.WithStack(err)
}

func (rt *RecordTransactor) DeleteRecord(ctx context.Context, uri syntax.ATURI) error {
	return parallel.Do(ctx,
		func(ctx context.Context) error {
			_, err := rt.db.ExecContext(ctx, `DELETE FROM record WHERE uri = ?`, uri)
			return errors.WithStack(err)
		},
		func(ctx context.Context) error {
			_, err := rt.db.ExecContext(ctx, `DELETE FROM backlink WHERE uri = ?`, uri)
			return errors.WithStack(err)
		})
}

type RepoRecord map[string]any

func (r RepoRecord) Type() (string, bool) {
	t, ok := r["$type"]
	if !ok {
		return "", false
	}
	tp, valid := t.(string)
	return tp, valid
}

func (r RepoRecord) Subject() (any, bool) {
	s, ok := r["subject"]
	return s, ok
}

type Backlink struct {
	URI    string `json:"uri"`
	Path   string `json:"path"`
	LinkTo string `json:"linkTo"`
}

// LinkableRecord is a type that is stored as json and has a "$type" field and a
// "subject" field.
type LinkableRecord interface {
	// Type should return the "$type" field.
	Type() (id string, found bool)
	// Subject should return the record's subject.
	Subject() (subj any, found bool)
}

func getBacklinks(uri syntax.ATURI, record LinkableRecord) []Backlink {
	if record == nil {
		return nil
	}
	recordType, ok := record.Type()
	if !ok {
		return nil
	}

	// Handle follow and block
	if recordType == "app.bsky.graph.follow" || recordType == "app.bsky.graph.block" {
		subj, ok := record.Subject()
		if !ok {
			return nil
		}
		subject, ok := subj.(string)
		if !ok {
			return nil
		}
		if err := ensureValidDid(subject); err != nil {
			return nil
		}
		return []Backlink{{
			URI:    uri.String(),
			Path:   "subject",
			LinkTo: subject,
		}}
	}

	// Handle like and repost
	if recordType == "app.bsky.feed.like" || recordType == "app.bsky.feed.repost" {
		subj, ok := record.Subject()
		if !ok {
			return nil
		}
		subject, ok := subj.(map[string]any)
		if !ok {
			return nil
		}
		subjectUri, ok := subject["uri"].(string)
		if !ok {
			return nil
		}
		if err := ensureValidAtUri(subjectUri); err != nil {
			return nil
		}
		return []Backlink{{
			URI:    uri.String(),
			Path:   "subject.uri",
			LinkTo: subjectUri,
		}}
	}
	return nil
}

func ensureValidDid(did string) error {
	_, err := syntax.ParseDID(did)
	return err
}

func ensureValidAtUri(uri string) error {
	_, err := syntax.ParseATURI(uri)
	return err
}

func strMapProcess(m map[string]any) (map[string]any, error) {
	var err error
	for k, v := range m {
		if mval, ok := v.(map[any]any); ok {
			v, err = toStringMapKeys(mval)
			if err != nil {
				return nil, err
			}
			m[k] = v
		}
	}
	return m, nil
}

func toStringMapKeys(m map[any]any) (map[string]any, error) {
	res := make(map[string]any)
	var err error
	for k, v := range m {
		var val any
		if vv, ok := v.(map[any]any); ok {
			val, err = toStringMapKeys(vv)
			if err != nil {
				return nil, err
			}
		} else {
			val = v
		}
		if key, ok := k.(string); ok {
			res[key] = val
		} else {
			return nil, errors.New("failed to change key any to string")
		}
	}
	return res, nil
}

var decMode cbor.DecMode

func init() {
	var err error
	tags := cbor.NewTagSet()
	err = tags.Add(
		cbor.TagOptions{
			EncTag: cbor.EncTagRequired,
			DecTag: cbor.DecTagOptional,
		},
		reflect.TypeOf(cid.Cid{}),
		42,
	)
	if err != nil {
		panic(err)
	}
	decMode, err = cbor.DecOptions{
		MapKeyByteString: cbor.MapKeyByteStringForbidden,
		DefaultMapType:   reflect.TypeOf(map[string]any{}),
	}.DecModeWithTags(tags)
	if err != nil {
		panic(err)
	}
}
