package actorstore

import (
	"context"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/huandu/go-sqlbuilder"
	"github.com/ipfs/go-cid"
	"github.com/matryer/is"
	_ "github.com/mattn/go-sqlite3"

	comatp "github.com/harrybrwn/at/api/com/atproto"
	"github.com/harrybrwn/at/internal/repo"
)

func TestGetRecord(t *testing.T) {
	is := is.New(t)
	ctx := t.Context()
	did := syntax.DID("did:plc:kzvsijt4365vidgqv7o6wksi")
	as := ActorStore{Dir: "./testdata/0/actors", ReadOnly: true}
	rr, err := as.Record(did)
	is.NoErr(err)
	defer rr.Close()
	record, err := rr.GetRecord(
		ctx,
		syntax.ATURI("at://did:plc:kzvsijt4365vidgqv7o6wksi/app.bsky.feed.post/3lcdxigdxbs2v"),
		ptr(cid.MustParse("bafyreiatwnfkxm53e3puyimx45ftesimjzku5xkotmxz6g6fu6dyygqkdy")),
		false,
	)
	is.NoErr(err)
	value, ok := record.Value.(map[string]any)
	is.True(ok)
	typ, ok := value["$type"]
	is.True(ok)
	is.Equal(typ, "app.bsky.feed.post")
	is.Equal(value["text"], "come to the dark side, we secretly like being bullied by the borrow checker")
	embed, ok := value["embed"].(map[string]any)
	is.True(ok && embed != nil)
	is.Equal(embed["$type"], "app.bsky.embed.images")
	reply, ok := value["reply"].(map[string]any)
	is.True(ok && reply != nil)
	uri, ok := reply["parent"].(map[string]any)["uri"]
	is.True(ok)
	is.Equal(uri, "at://did:plc:6wjd7vrkvfw6dsboypufmy7d/app.bsky.feed.post/3lcbuxjxcgc2u")
	is.Equal(reply["root"].(map[string]any)["uri"], "at://did:plc:6wjd7vrkvfw6dsboypufmy7d/app.bsky.feed.post/3lcbuxjxcgc2u")
	langs, ok := value["langs"]
	is.True(ok && langs != nil)
	is.Equal([]any{"en"}, langs)
}

func TestListForCollection(t *testing.T) {
	is := is.New(t)
	ctx := t.Context()
	did := syntax.DID("did:plc:kzvsijt4365vidgqv7o6wksi")
	as := ActorStore{Dir: "./testdata/0/actors", ReadOnly: true}
	rr, err := as.Record(did)
	is.NoErr(err)
	defer rr.Close()
	res, err := rr.ListForCollection(ctx, &comatp.RepoListRecordsParams{
		// Collection: "app.bsky.feed.like",
		Collection: "app.bsky.feed.post",
	})
	is.NoErr(err)
	is.True(len(res.Records) > 0)
	// for _, rec := range res.Records {
	// 	fmt.Printf("%#v\n", rec)
	// }
}

func TestPutRecord(t *testing.T) {
	is := is.New(t)
	ctx := t.Context()
	did := syntax.DID("did:plc:kzvsijt4365vidgqv7o6wksi")
	as := testStore(t)
	rr, err := as.CreateAsRecord(did, testKey(t))
	is.NoErr(err)
	defer rr.Close()
	record := map[string]any{
		"$type": "me.hrry.blog.post",
		"title": "Test Post",
		"body": `The AT Protocol is kinda cool. One reason is that its sort
of a generic database of records attached to your identity.`,
		"createdAt": time.Now().Format(time.RFC3339),
	}
	// res, err := rr.PutRecord(ctx, &atproto.RepoPutRecordRequest{
	// 	Collection: syntax.NSID("me.hrry.blog.post"),
	// 	Repo:       &syntax.AtIdentifier{Inner: did},
	// 	RKey:       "1",
	// 	Record:     record,
	// })
	// is.NoErr(err)
	// _ = res

	err = rr.Tx(ctx, func(ctx context.Context, tx *RecordTransactor) error {
		return tx.IndexRecord(
			ctx,
			syntax.ATURI("at://"+did+"/me.hrry.blog.post/1"),
			cid.Cid{},
			record,
			repo.WriteOpActionCreate,
			"",
			time.Now(),
		)
	})
	is.NoErr(err)
}

func TestSQLBuilder(t *testing.T) {
	is := is.New(t)
	collection := "app.bsky.feed.post"
	cursor := "4"
	qb := sqlbuilder.SQLite.NewSelectBuilder()
	qb.Select("record.uri", "record.cid", "repo_block.content")
	qb.From("record").JoinWithOption(sqlbuilder.InnerJoin, "repo_block", "repo_block.cid = record.cid")
	qb.Where(
		qb.Equal("record.collection", collection),
		"record.takedownRef is null",
	)
	if len(cursor) > 0 {
		qb.Where(qb.GreaterThan("record.rkey", cursor))
	}
	qb.OrderBy("record.rkey").Desc()
	query, args := qb.Build()
	is.Equal(args, []any{collection, cursor})
	is.True(len(query) > 0)
}

func ptr[T any](v T) *T { return &v }
