package actorstore

import (
	"context"
	"fmt"
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
	// t.Skip()
	is := is.New(t)
	ctx := context.Background()
	did := syntax.DID("did:plc:kzvsijt4365vidgqv7o6wksi")
	as := ActorStore{Dir: "./testdata/0/actors", ReadOnly: true}
	rr, err := as.Record(did)
	is.NoErr(err)
	defer rr.Close()

	// collections, err := rr.ListCollections(ctx)
	// is.NoErr(err)
	// for c, err := range collections {
	// 	is.NoErr(err)
	// 	fmt.Println(c)
	// }
	record, err := rr.GetRecord(
		ctx,
		// syntax.ATURI("at://did:plc:kzvsijt4365vidgqv7o6wksi/app.bsky.feed.post/3lcnr6kiowc2i"),
		// ptr(syntax.CID("bafyreiach5hxu7eatczmb3p4r64j2l27gtz2d4kicd4ejxmbbn2ffwfzqy")),
		// syntax.ATURI("at://did:plc:kzvsijt4365vidgqv7o6wksi/app.bsky.feed.like/3lcbpvnpox22z"),
		// ptr(syntax.CID("bafyreier4z455ysz227gs2xoqacsst5odfr5zoqg4v3javj363r7ffxiye")),
		// syntax.ATURI("at://did:plc:kzvsijt4365vidgqv7o6wksi/app.bsky.actor.profile/self"),
		// ptr(syntax.CID("bafyreibjy6ikgxb6yqum5u6q4bf7ehacogpqybde3pbkp3idqwhkpqcc2i")),
		syntax.ATURI("at://did:plc:kzvsijt4365vidgqv7o6wksi/app.bsky.feed.post/3lcdxigdxbs2v"),
		ptr(cid.MustParse("bafyreiatwnfkxm53e3puyimx45ftesimjzku5xkotmxz6g6fu6dyygqkdy")),
		false,
	)
	if err != nil {
		fmt.Printf("%+v\n", err)
		is.Fail()
	}
	is.NoErr(err)
	// fmt.Printf("record: %#v\n", record)
	value, ok := record.Value.(map[string]any)
	is.True(ok)
	fmt.Printf("value: %#v\n", value)
	_, ok = value["avatar"].(map[string]any)
	is.True(ok)
}

func TestListForCollection(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
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
	for _, rec := range res.Records {
		fmt.Printf("%#v\n", rec)
	}
}

func TestPutRecord(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
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

	fmt.Println(args)
	fmt.Println(query)
}

func ptr[T any](v T) *T { return &v }
