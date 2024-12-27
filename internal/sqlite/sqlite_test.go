package sqlite

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/harrybrwn/db"
	"github.com/matryer/is"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func TestConfig(t *testing.T) {
	is := is.New(t)
	u := url.URL{
		Scheme:   "file",
		Path:     filepath.Join(t.TempDir(), "test.sqlite"),
		RawQuery: must((&Config{Cache: CacheModePrivate}).query()).Encode(),
	}
	d, err := sql.Open("sqlite3", u.String())
	is.NoErr(err)
	defer d.Close()
	u = url.URL{
		Opaque:   ":memory:",
		RawQuery: "cache=shared",
	}
	is.Equal(u.String(), ":memory:?cache=shared")
	c := Config{
		ReadOnly: true,
		Cache:    CacheModeShared,
	}
	q, err := c.query()
	is.NoErr(err)
	is.Equal(q.Get("mode"), "ro")
	is.Equal(q.Get("immutable"), "true")
	is.Equal(q.Get("cache"), "shared")
}

func TestOpen(t *testing.T) {
	is := is.New(t)
	d, err := Open(
		filepath.Join(t.TempDir(), "test.sqlite"),
		&Config{
			JournalMode: "TRUNCATE",
		},
	)
	is.NoErr(err)
	defer d.Close()
	mode, err := GetJournalMode(db.Simple(d))
	is.NoErr(err)
	is.Equal(mode, "truncate")
}

func TestInMemory(t *testing.T) {
	is := is.New(t)
	d, err := InMemory()
	is.NoErr(err)
	defer d.Close()
	mode, err := GetJournalMode(db.Simple(d))
	is.NoErr(err)
	is.Equal(mode, "memory")
}

func TestPragmas(t *testing.T) {
	is := is.New(t)
	uri := url.URL{
		Scheme: "file",
		Opaque: filepath.Join(t.TempDir(), "test.db"),
	}
	d, err := open(&uri, nil)
	is.NoErr(err)
	defer d.Close()
	db := db.Simple(d)
	mode, err := GetJournalMode(db)
	is.NoErr(err)
	is.Equal(mode, "delete") // delete should be the default
	c := Config{
		JournalMode:   "WAL",
		WalCheckpoint: ptr(7), // TODO test the results of this
	}
	WithPragma(PragmaSynchronous, SynchronousExtra)(&c)
	WithPragma(PragmaCacheSize, 69)(&c)
	err = c.pragmas(d)
	if err != nil {
		fmt.Printf("%+v\n", err)
	}
	is.NoErr(err)
	mode, err = GetJournalMode(db)
	is.NoErr(err)
	is.Equal(mode, "wal")
	sync, err := GetPragmaSynchronous(db)
	is.NoErr(err)
	is.Equal(sync, SynchronousExtra)
	cacheSize, err := GetPragmaCacheSize(db)
	is.NoErr(err)
	is.Equal(cacheSize, int64(69))
	databases, err := GetPragmaDatabaseList(db)
	is.NoErr(err)
	is.Equal(len(databases), 1)
	is.Equal(databases[0].Index, 0)
	is.Equal(databases[0].Name, "main")
	is.Equal(databases[0].Location, uri.Opaque)
	_, _, _, err = GetWalCheckpoint(db) // TODO check values
	is.NoErr(err)
}

func TestListTables(t *testing.T) {
	is := is.New(t)
	d, err := File(
		filepath.Join(t.TempDir(), "test.sqlite"),
		Cache(CacheModePrivate),
		JournalMode("WAL"),
		WalCheckpoint(0),
		debug,
	)
	if err != nil {
		fmt.Printf("%+v\n", err)
	}
	is.NoErr(err)
	defer d.Close()
	_, err = d.Exec(`CREATE TABLE testing_table (
		name varchar,
		number INT
	)`)
	is.NoErr(err)
	names, err := ListTablesNames(db.Simple(d))
	is.NoErr(err)
	is.Equal(names, []string{"testing_table"})
}

func debug(c *Config) { c.Debug = true }

func must[T any](v T, e error) T {
	if e != nil {
		panic(e)
	}
	return v
}

func ptr[T any](v T) *T { return &v }
