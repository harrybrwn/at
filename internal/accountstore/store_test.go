package accountstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"os"
	"testing"
	"time"

	database "github.com/harrybrwn/db"
	"github.com/matryer/is"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"github.com/whyrusleeping/go-did"
)

func Test(t *testing.T) {
}

func TestAccountStore_CreateAccount(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	db, err := sql.Open("sqlite3", ":memory:")
	is.NoErr(err)
	defer db.Close()
	as := New(db, nil, "")
	is.NoErr(as.Migrate(ctx))
	_ = ctx
	did := newDID()
	access, refresh, err := as.CreateAccount(ctx, CreateAccountOpts{
		DID:      did,
		Handle:   "test.local",
		Email:    p("test@test.local"),
		Password: p("testlab01"),
	})
	is.NoErr(err)
	is.True(len(access) > 0)
	is.True(len(refresh) > 0)
	acct, err := as.GetAccount(ctx, did, nil)
	is.NoErr(err)
	is.True(acct != nil)
	var handle string
	rows, err := db.Query("select handle from actor where did = ?", did)
	is.NoErr(err)
	is.True(rows.Next())
	is.NoErr(rows.Scan(&handle))
	is.NoErr(rows.Close())
	is.Equal(handle, "test.local")
	is.True(acct.Handle.Valid) // should not be null
	is.Equal(acct.Handle.String, handle)
	rows, err = db.Query("select email from account where did = ?", did)
	is.NoErr(err)
	is.True(rows.Next())
	is.NoErr(rows.Scan(&handle))
	is.NoErr(rows.Close())
	is.Equal(handle, "test@test.local")
	is.Equal(acct.Email, handle)
	pw, err := getAccountPassword(ctx, database.Simple(db), did)
	is.NoErr(err)
	is.NoErr(verifyPassword("testlab01", pw))
	refreshTokenData, err := getRefreshTokenByDID(ctx, database.Simple(db), did)
	is.NoErr(err)
	is.True(len(refreshTokenData.ID) > 0)
	is.True(refreshTokenData.ExpiresAt != 0)
	rows, err = db.Query("select cid, rev, indexedAt from repo_root where did = ?", did)
	is.NoErr(err)
	var cid, rev, indexedAtStr string
	is.NoErr(database.ScanOne(rows, &cid, &rev, &indexedAtStr))
	indexedAt, err := time.Parse(time.RFC3339, indexedAtStr)
	is.NoErr(err)
	is.Equal(indexedAt.Minute(), time.Now().UTC().Minute())
	is.Equal(indexedAt.Hour(), time.Now().UTC().Hour())
	is.Equal(indexedAt.Day(), time.Now().UTC().Day())
	is.Equal(indexedAt.Month(), time.Now().UTC().Month())
	is.Equal(indexedAt.Year(), time.Now().UTC().Year())
}

func TestCreateAccount_Func(t *testing.T) {
	did := os.Getenv("BSKY_TEST_DID")
	if len(did) == 0 {
		t.Skip()
	}

	// is := is.New(t)
	// ctx := context.Background()
	// db, err := sql.Open("sqlite3", "file:testdata/2/account.sqlite?mode=ro&immutable=true")
	// is.NoErr(err)
	// defer db.Close()
	// as := New(db, nil, "")
	// acct, err := as.GetAccount(ctx, did)
	// is.NoErr(err)
	// fmt.Printf("test: %+v\n", acct)

	// rows, err := db.Query("select cid, rev, indexedAt from repo_root where did = ?", did)
	// is.NoErr(err)
	// var cid, rev, indexedAt string
	// is.NoErr(database.ScanOne(rows, &cid, &rev, &indexedAt))
	// fmt.Println("cid:", cid)
	// fmt.Println("rev:", rev)
	// fmt.Println("indexedAt:", indexedAt)
}

func TestHashPassword(t *testing.T) {
	hash, err := hashPassword("testbench01")
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) == 0 {
		t.Error("hash has zero length")
	}
	if !bytes.Contains(hash, []byte{':'}) {
		t.Error("expetced ':' to be contained in the password hash")
	}
	if err = verifyPassword("testbench01", hash); err != nil {
		t.Error(err)
	}
}

func TestVerifyPassword_Func(t *testing.T) {
	did := os.Getenv("BSKY_TEST_DID")
	password := os.Getenv("BSKY_TEST_PASSWORD")
	if len(did) == 0 || len(password) == 0 {
		t.Skip()
	}
	is := is.New(t)
	ctx := context.Background()
	db, err := sql.Open("sqlite3", "file:testdata/2/account.sqlite?mode=ro&immutable=true")
	is.NoErr(err)
	defer db.Close()
	storedPwHash, err := getAccountPassword(ctx, database.Simple(db), did)
	is.NoErr(err)
	is.NoErr(verifyPassword(password, storedPwHash))
}

func TestCreateSession(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	db, err := sql.Open("sqlite3", ":memory:")
	is.NoErr(err)
	defer db.Close()
	as := New(db, nil, "")
	is.NoErr(as.Migrate(ctx))
	_ = ctx
	did := newDID()
	ap, err := createAppPassword(ctx, database.New(db), did, "test ap", true)
	is.NoErr(err)
	access, refresh, err := as.CreateSession(ctx, did, &AppPassDescript{Name: ap.Name, Privileged: ap.Privileged})
	is.NoErr(err)
	is.True(len(access) > 0)
	is.True(len(refresh) > 0)
}

func getAccountPassword(ctx context.Context, db database.DB, identifier string) ([]byte, error) {
	rows, err := db.QueryContext(ctx, "SELECT passwordScrypt FROM account WHERE did = ?", identifier)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var password string
	if err = database.ScanOne(rows, &password); err != nil {
		return nil, errors.WithStack(err)
	}
	return []byte(password), nil
}

func newDID() string {
	key, err := did.GeneratePrivKey(rand.Reader, did.KeyTypeSecp256k1)
	if err != nil {
		panic(err)
	}
	did, err := did.DIDFromKey(key.Public().Raw)
	if err != nil {
		panic(err)
	}
	return did.String()
}

func p[T any](v T) *T { return &v }
