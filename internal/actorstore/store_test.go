package actorstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/huandu/go-sqlbuilder"

	"github.com/harrybrwn/at/array"
)

func testStore(t *testing.T) *ActorStore {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "actors")
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	return &ActorStore{Dir: dir, ReadOnly: false}
}

func testKey(t *testing.T) *crypto.PrivateKeyK256 {
	t.Helper()
	k, err := crypto.GeneratePrivateKeyK256()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestQueryBuilder(t *testing.T) {
	qb := sqlbuilder.SQLite.NewSelectBuilder()
	cidStrs := []string{
		"bafyreichqj7fpw5h6kkxnygcdaccq5ey7n43kpy46mqqu6v6du74k6ujfm",
		"bafyreienq3lbn7xfwfemtaytowdmnmetfrviba6kchu3xdom35adnygekm",
		"bafyreienq3lbn7xfwfemtaytowdmnmetfrviba6kchu3xdom35adnygekm",
	}
	uriStrs := []string{
		"at://did:plc:x2nsupeeo52oznrmplwapppl/app.bsky.feed.post/3leqr75pg5s2a",
		"at://did:plc:fpruhuo22xkm5o7ttr2ktxdo/app.bsky.feed.post/3ldpjakg72k2j",
	}
	qb.Select("cid").
		From("reocrd").
		Where(
			qb.In("cid", array.Map(cidStrs, array.ToAny)...),
			qb.Not(qb.In("uri", array.Map(uriStrs, array.ToAny)...)),
		)
	query, args := qb.Build()
	exp := "SELECT cid FROM reocrd WHERE cid IN (?, ?, ?) AND NOT uri IN (?, ?)"
	if query != exp {
		t.Errorf("expected query %q, got %q", exp, query)
	}
	if len(args) <= 2 {
		t.Error("invalid number of args")
	}
	if len(args) != len(cidStrs)+len(uriStrs) {
		t.Error("invalid number of args")
	}
}
