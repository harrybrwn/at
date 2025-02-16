package didcache

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/matryer/is"
	"github.com/whyrusleeping/go-did"
)

var sampleDoc = did.Document{
	Context: []string{
		"https://www.w3.org/ns/did/v1",
		"https://w3id.org/security/multikey/v1",
		"https://w3id.org/security/suites/secp256k1-2019/v1",
	},
	ID:          must(did.ParseDID("did:plc:ewvi7nxzyoun6zhxrhs64oiz")),
	AlsoKnownAs: []string{"handle.txt"},
	VerificationMethod: []did.VerificationMethod{
		{
			ID:                 "did:plc:ewvi7nxzyoun6zhxrhs64oiz#atproto",
			Type:               "Multikey",
			Controller:         "did:plc:ewvi7nxzyoun6zhxrhs64oiz",
			PublicKeyMultibase: ptr("zQ3shunBKsXixLxKtC5qeSG9E4J5RkGN57im31pcTzbNQnm5w"),
		},
	},
	Service: []did.Service{
		{
			ID:              must(did.ParseDID("#atproto_pds")),
			Type:            "AtprotoPersonalDataServer",
			ServiceEndpoint: "https://enoki.us-east.host.bsky.network",
		},
	},
}

func TestDocCache(t *testing.T) {
	ctx := t.Context()
	is := is.New(t)
	c, err := NewDIDCache(":memory:", time.Second, time.Minute)
	is.NoErr(err)
	defer c.Close()
	res, err := c.CachedDoc(ctx, "did:plc:ewvi7nxzyoun6zhxrhs64oiz")
	is.True(err != nil)
	is.True(errors.Is(err, sql.ErrNoRows))
	is.True(res == nil)
	err = c.CacheDoc(ctx, "did:plc:ewvi7nxzyoun6zhxrhs64oiz", &sampleDoc, nil)
	is.NoErr(err)
	res, err = c.CachedDoc(ctx, "did:plc:ewvi7nxzyoun6zhxrhs64oiz")
	is.NoErr(err)
	is.True(res != nil)
	is.True(!res.Stale)
	is.True(!res.Expired)
	is.Equal(res.Doc, sampleDoc)
	is.NoErr(c.ClearEntry(ctx, "did:plc:ewvi7nxzyoun6zhxrhs64oiz"))
}

func TestDIDCache(t *testing.T) {
	ctx := t.Context()
	is := is.New(t)
	c, err := NewDIDCache("file::memory:?cache=shared", time.Second, time.Minute)
	is.NoErr(err)
	defer c.Close()
	printDB(io.Discard, c.db)

	_, err = c.GetDID(ctx, sampleDoc.AlsoKnownAs[0])
	is.True(err != nil)
	err = c.StoreDID(ctx, sampleDoc.AlsoKnownAs[0], sampleDoc.ID.String())
	is.NoErr(err)
	res, err := c.GetDID(ctx, sampleDoc.AlsoKnownAs[0])
	is.NoErr(err)
	is.Equal(res.DID, sampleDoc.ID.String())
	is.True(!res.Expired)
	is.True(!res.Stale)

	time.Sleep(time.Millisecond * 1) // make sure updatedAt is different
	err = c.StoreDID(ctx, sampleDoc.AlsoKnownAs[0], sampleDoc.ID.String())
	is.NoErr(err)
	res2, err := c.GetDID(ctx, sampleDoc.AlsoKnownAs[0])
	is.NoErr(err)
	is.True(res2.UpdatedAt.After(res.UpdatedAt))
}

func TestIdentityCache(t *testing.T) {
	ctx := t.Context()
	is := is.New(t)
	c, err := NewDIDCache("file::memory:?cache=shared", time.Second, time.Minute)
	is.NoErr(err)
	defer c.Close()
	did := "did:plc:ewvi7nxzyoun6zhxrhs64oiz"
	ident := identity.Identity{
		DID:         syntax.DID("did:plc:ewvi7nxzyoun6zhxrhs64oiz"),
		Handle:      syntax.Handle("sample.txt"),
		AlsoKnownAs: []string{"sample.txt"},
		Services: map[string]identity.Service{
			"#atproto_pds": {Type: "AtprotoPersonalDataServer", URL: "https://enoki.us-east.host.bsky.network"},
		},
		Keys: map[string]identity.Key{
			"did:plc:ewvi7nxzyoun6zhxrhs64oiz#atproto": {
				Type:               "Multikey",
				PublicKeyMultibase: "zQ3shunBKsXixLxKtC5qeSG9E4J5RkGN57im31pcTzbNQnm5w",
			},
		},
	}
	is.NoErr(c.StoreIdentity(ctx, did, &ident))
	res, err := c.GetIdentity(ctx, did)
	is.NoErr(err)
	is.Equal(res.Ident, ident)
	is.True(!res.Expired)
	is.True(!res.Stale)
	time.Sleep(time.Millisecond) // make sure updatedAt is different
	is.NoErr(c.StoreIdentity(ctx, did, &ident))
	res2, err := c.GetIdentity(ctx, did)
	is.NoErr(err)
	is.True(res2.UpdatedAt.After(res.UpdatedAt))
	is.NoErr(c.ClearEntry(ctx, did))
	_, err = c.GetIdentity(ctx, did)
	is.True(err != nil)
}

func ptr[T any](v T) *T { return &v }

func must[T any](v T, e error) T {
	if e != nil {
		panic(e)
	}
	return v
}

func printDB(w io.Writer, db *sql.DB) {
	rows, err := db.Query(`SELECT name, sql FROM sqlite_master
						   WHERE type='table' ORDER BY name`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var name, body string
		err = rows.Scan(&name, &body)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(w, "%q %s\n", name, body)
	}
	fmt.Fprintln(w)
}
