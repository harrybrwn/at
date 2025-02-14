package actorstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/harrybrwn/db"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/internal/repo"
)

//go:embed init.sql
var migration []byte

type ActorStore struct {
	Dir      string
	ReadOnly bool
}

type datastore struct {
	db  db.DB
	did syntax.DID
	key *crypto.PrivateKeyK256
}

func (ds *datastore) Close() error {
	return ds.db.Close()
}

func (as *ActorStore) Create(did syntax.DID, signingKey crypto.PrivateKeyExportable) error {
	ds, err := as.migrate(did, signingKey)
	if err != nil {
		return err
	}
	return ds.Close()
}

func (as *ActorStore) CreateAsPref(did syntax.DID, key crypto.PrivateKeyExportable) (*PreferenceReader, error) {
	ds, err := as.migrate(did, key)
	return (*PreferenceReader)(ds), err
}

func (as *ActorStore) CreateAsRecord(did syntax.DID, key crypto.PrivateKeyExportable) (*RecordReader, error) {
	ds, err := as.migrate(did, key)
	return (*RecordReader)(ds), err
}

func (as *ActorStore) CreateAsRepo(did syntax.DID, key crypto.PrivateKeyExportable) (*SQLRepoReader, error) {
	ds, err := as.migrate(did, key)
	if err != nil {
		return nil, err
	}
	return NewSQLRepoReader(ds.db, did, key), err
}

func (as *ActorStore) Pref(did syntax.DID) (r *PreferenceReader, err error) {
	ds, err := as.datastore(did)
	return (*PreferenceReader)(ds), err
}

func (as *ActorStore) Record(did syntax.DID) (*RecordReader, error) {
	ds, err := as.datastore(did)
	return (*RecordReader)(ds), err
}

func (as *ActorStore) Repo(did syntax.DID, key crypto.PrivateKeyExportable) (*SQLRepoReader, error) {
	ds, err := as.datastore(did)
	if err != nil {
		return nil, err
	}
	return NewSQLRepoReader(ds.db, did, key), nil
}

func (as *ActorStore) Destroy(did syntax.DID) error {
	// TODO remove blobs in blobstore (also add a blobstore)
	dir, _, _ := as.location(did)
	return os.RemoveAll(dir)
}

func (as *ActorStore) datastore(did syntax.DID) (ds *datastore, err error) {
	ds = new(datastore)
	_, dbpath, keypath := as.location(did)
	database, err := as.openDB(dbpath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open database %q", dbpath)
	}
	ds.db = db.Simple(database)
	ds.key, err = as.key(keypath)
	if err != nil {
		database.Close()
		return nil, errors.Wrapf(err, "failed to open private key %q", keypath)
	}
	return ds, nil
}

func (as *ActorStore) openDB(path string) (*sql.DB, error) {
	q := make(url.Values)
	if as.ReadOnly {
		q.Set("mode", "ro")
		q.Set("immutable", "true")
	}
	uri := url.URL{
		Path:     path,
		RawQuery: q.Encode(),
	}
	db, err := sql.Open("sqlite3", "file:"+uri.String())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	_, err = db.Exec(`PRAGMA journal_mode = WAL`)
	return db, errors.WithStack(err)
}

func (as *ActorStore) key(path string) (*crypto.PrivateKeyK256, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	k, err := crypto.ParsePrivateBytesK256(b)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return k, nil
}

func sha256Hex(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func (as *ActorStore) migrate(did syntax.DID, signingKey crypto.PrivateKeyExportable) (*datastore, error) {
	if len(did) == 0 {
		return nil, errors.Errorf("invalid did %q", did)
	}
	dir, dbpath, keypath := as.location(did)
	_ = os.MkdirAll(dir, 0755)
	database, err := as.openDB(dbpath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	_, err = database.Exec(string(migration))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	keyfile, err := os.OpenFile(keypath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer keyfile.Close()
	_, err = keyfile.Write(signingKey.Bytes())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &datastore{
		db:  db.Simple(database),
		did: did,
		key: signingKey.(*crypto.PrivateKeyK256),
	}, nil
}

func (as *ActorStore) location(did syntax.DID) (dir, db, key string) {
	hash := sha256Hex(did.String())
	dir = filepath.Join(as.Dir, hash[:2], did.String())
	db = filepath.Join(dir, "store.sqlite")
	key = filepath.Join(dir, "key")
	return dir, db, key
}

func RunMigration(d *sql.DB) error {
	_, err := d.Exec(string(migration))
	return err
}

type ActorStoreTransactor struct {
	Repo   *RepoTransactor
	Record *RecordTransactor
	Pref   *PreferenceReader
}

func (as *ActorStore) Transact(ctx context.Context, did syntax.DID, blobs repo.BlobStore, fn func(ctx context.Context, tx *ActorStoreTransactor) error) error {
	ds, err := as.datastore(did)
	if err != nil {
		return err
	}
	defer ds.Close()
	tx, err := ds.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	now := time.Now()
	txds := datastore{
		db:  tx,
		did: ds.did,
		key: ds.key,
	}
	err = fn(ctx, &ActorStoreTransactor{
		Repo: NewRepoTransactor(
			tx,
			ds.did,
			ds.key,
			blobs,
			nil,
			now.UTC().Format(time.RFC1123),
		),
		Record: &RecordTransactor{RecordReader: (*RecordReader)(&txds)},
		Pref:   (*PreferenceReader)(&txds),
	})
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
