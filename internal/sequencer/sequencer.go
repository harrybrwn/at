package sequencer

import (
	"context"
	"database/sql"
	_ "embed"
	"sync/atomic"
	"time"

	database "github.com/harrybrwn/db"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/internal/cbor/dagcbor"
	"github.com/harrybrwn/at/internal/repo"
	"github.com/harrybrwn/at/internal/sqlite"
	"github.com/harrybrwn/at/pubsub"
)

//go:embed init.sql
var migration []byte

type SeqSetter interface {
	SetSeq(seq int64)
}

type Bus[T SeqSetter] = pubsub.Bus[pubsub.Empty, *Event[T]]

type Seq[T SeqSetter] struct {
	db       *sql.DB
	bus      Bus[T]
	lastSeen int64
	seq      atomic.Int64
}

type Event[T any] struct {
	Seq         int64
	EventType   repo.EventType
	Event       T
	SequencedAt time.Time
}

type RepoSeq struct {
	Seq         int64          `json:"seq" cbor:"seq"`                 // Primary key, auto-increment
	DID         string         `json:"did" cbor:"did"`                 // Not null
	EventType   repo.EventType `json:"eventType" cbor:"eventType"`     // Not null
	Event       []byte         `json:"event" cbor:"event"`             // Blob, not null
	Invalidated bool           `json:"invalidated" cbor:"invalidated"` // Default 0, not null
	SequencedAt time.Time      `json:"sequencedAt" cbor:"sequencedAt"` // Not null
}

func New[T SeqSetter](location string, bus Bus[T]) (*Seq[T], error) {
	db, err := sqlite.File(location, sqlite.JournalMode("WAL"), sqlite.WalCheckpoint(0))
	if err != nil {
		return nil, err
	}
	s := Seq[T]{
		db:  db,
		bus: bus,
	}
	ctx := context.Background()
	err = s.migrate(ctx)
	if err != nil {
		return nil, err
	}
	s.lastSeen, err = s.curr(ctx)
	if err != nil {
		s.lastSeen = 0
	}
	s.seq.Store(s.lastSeen)
	return &s, nil
}

func (s *Seq[T]) Pub(ctx context.Context, evt *Event[T]) error {
	now := time.Now()
	pub, err := s.bus.Publisher(ctx)
	if err != nil {
		return err
	}
	n := s.seq.Add(1)
	evt.Event.SetSeq(n)
	evt.SequencedAt = now
	err = pub.Pub(ctx, evt)
	if err != nil {
		return err
	}
	data, err := dagcbor.Marshal(evt)
	if err != nil {
		return errors.WithStack(err)
	}
	err = s.store(ctx, &RepoSeq{
		Seq:         n,
		Event:       data,
		SequencedAt: now,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *Seq[T]) Subscriber(ctx context.Context, _ ...pubsub.Empty) (pubsub.Sub[*Event[T]], error) {
	return s.bus.Subscriber(ctx)
}

func (s *Seq[T]) Publisher(ctx context.Context, _ ...pubsub.Empty) (pubsub.Pub[*Event[T]], error) {
	return &publisher[T]{Seq: s}, nil
}

type publisher[T SeqSetter] struct{ *Seq[T] }

func (p *publisher[T]) Close() error { return nil }

func (s *Seq[T]) Close() error {
	_ = s.bus.Close()
	return s.db.Close()
}

func (s *Seq[T]) store(ctx context.Context, e *RepoSeq) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO repo_seq (
		seq,
		did,
		eventType,
		event,
		invalidated,
		sequencedAt
	) VALUES (?,?,?,?,?,?)`,
		e.Seq,
		e.DID,
		e.EventType,
		e.Event,
		e.Invalidated,
		e.SequencedAt.Format(time.RFC3339),
	)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (s *Seq[T]) curr(ctx context.Context) (int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT seq FROM repo_seq ORDER BY seq DESC LIMIT 1`)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	var seq int64
	if err = database.ScanOne(rows, &seq); err != nil {
		return 0, errors.WithStack(err)
	}
	return seq, nil
}

func (s *Seq[T]) next(ctx context.Context, cursor int) (*RepoSeq, error) {
	var rs RepoSeq
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT `+repoSeqSelectHead+
			` FROM repo_seq WHERE seq > ? ORDER BY seq ASC LIMIT 1`,
		cursor,
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = scanRepoSeq(rows, &rs)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &rs, nil
}

func (s *Seq[T]) earliestAfterTime(ctx context.Context, t time.Time) (*RepoSeq, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+repoSeqSelectHead+
		` FROM repo_seq WHERE sequencedAt >= ? ORDER BY sequencedAt ASC LIMIT 1`, t)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var rs RepoSeq
	err = scanRepoSeq(rows, &rs)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &rs, nil
}

const repoSeqSelectHead = `seq, did, eventType, event, invalidated, sequencedAt`

func scanRepoSeq(rows database.Rows, rs *RepoSeq) error {
	return database.ScanOne(
		rows,
		&rs.Seq,
		&rs.DID,
		&rs.EventType,
		&rs.Event,
		&rs.Invalidated,
		&rs.SequencedAt,
	)
}

func (s *Seq[T]) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, string(migration))
	return errors.WithStack(err)
}

func NewEvent[T any](v T) *Event[T] {
	return &Event[T]{Event: v}
}
