package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/harrybrwn/db"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

func Open(location string, config *Config) (*sql.DB, error) {
	query, err := config.query()
	if err != nil {
		return nil, err
	}
	uri := url.URL{
		Scheme:   "file",
		Opaque:   location,
		RawQuery: query.Encode(),
	}
	return open(&uri, config)
}

func OpenURI(uri *url.URL, opts ...Option) (*sql.DB, error) {
	var config Config
	for _, o := range opts {
		o(&config)
	}
	query, err := config.query()
	if err != nil {
		return nil, err
	}
	uri.RawQuery = query.Encode()
	return open(uri, &config)
}

func File(location string, opts ...Option) (*sql.DB, error) {
	var config Config
	for _, o := range opts {
		o(&config)
	}
	query, err := config.query()
	if err != nil {
		return nil, err
	}
	uri := url.URL{
		Scheme:   "file",
		Opaque:   location,
		RawQuery: query.Encode(),
	}
	return open(&uri, &config)
}

func InMemory(opts ...Option) (*sql.DB, error) {
	var config Config
	for _, o := range opts {
		o(&config)
	}
	query, err := config.query()
	if err != nil {
		return nil, err
	}
	uri := url.URL{
		Opaque:   ":memory:",
		RawQuery: query.Encode(),
	}
	return open(&uri, &config)
}

func open(uri *url.URL, config *Config) (*sql.DB, error) {
	source := uri.String()
	if config != nil && config.Debug {
		slog.Debug("sql.Open", "driver", "sqlite3", "source", source)
	}
	db, err := sql.Open("sqlite3", source)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if config != nil {
		if err = config.pragmas(db); err != nil {
			return nil, err
		}
	}
	return db, nil
}

const (
	PragmaSynchronous    = "synchronous"
	PragmaJournalMode    = "journal_mode"
	PragmaWalCheckpoint  = "wal_checkpoint"
	PragmaCacheSize      = "cache_size"
	PragmaApplicationID  = "application_id"
	PragmaAutoVacuum     = "auto_vacuum"
	PragmaAutomaticIndex = "automatic_index"
	PragmaDataVersion    = "data_version"
	PragmaDatabaseList   = "database_list"
)

func GetPragma[T any](database db.DB, name string) (T, error) {
	var v T
	return v, getPragma(database, name, &v)
}

func GetJournalMode(database db.DB) (string, error) {
	return GetPragma[string](database, PragmaJournalMode)
}

func GetPragmaSynchronous(database db.DB) (Synchronous, error) {
	return GetPragma[Synchronous](database, PragmaSynchronous)
}

func GetPragmaCacheSize(database db.DB) (int64, error) {
	return GetPragma[int64](database, PragmaCacheSize)
}

func GetWalCheckpoint(database db.DB) (int, int, int, error) {
	var a, b, c int
	return a, b, c, getPragma(database, PragmaWalCheckpoint, &a, &b, &c)
}

type DatabaseList struct {
	Index    int
	Name     string
	Location string
}

func GetPragmaDatabaseList(database db.DB) ([]DatabaseList, error) {
	rows, err := database.QueryContext(
		context.Background(),
		`PRAGMA `+PragmaDatabaseList,
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()
	res := make([]DatabaseList, 0)
	for rows.Next() {
		var dl DatabaseList
		err = rows.Scan(&dl.Index, &dl.Name, &dl.Location)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		res = append(res, dl)
	}
	return res, nil
}

func getPragma(database db.DB, name string, dst ...any) error {
	rows, err := database.QueryContext(
		context.Background(),
		`PRAGMA `+name,
	)
	if err != nil {
		return err
	}
	return db.ScanOne(rows, dst...)
}

// See https://www.sqlite.org/pragma.html
type Synchronous uint8

const (
	SynchronousOff Synchronous = iota
	SynchronousNormal
	SynchronousFull
	SynchronousExtra
)

func ListTablesNames(db db.DB) ([]string, error) {
	rows, err := db.QueryContext(context.Background(), `SELECT tbl_name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := make([]string, 0)
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		names = append(names, name)
	}
	return names, nil
}

func exec(c *Config, database *sql.DB, query string, args ...any) error {
	if c.Debug {
		slog.Debug("EXEC", "query", query, "args", args)
	}
	_, err := database.Exec(query, args...)
	return errors.WithStack(err)
}

func pragma(c *Config, database *sql.DB, name string, value any) error {
	return exec(c, database, fmt.Sprintf("PRAGMA %s = %v", name, value))
}
