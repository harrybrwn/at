package pds

import (
	"context"
	"fmt"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/matryer/is"

	"github.com/harrybrwn/at/api/com/atproto"
	"github.com/harrybrwn/at/xrpc"
)

func TestDescribeRepo(t *testing.T) {
	t.Skip()
	is := is.New(t)
	pds := testPDS(t)
	ctx := context.Background()
	res, err := pds.DescribeRepo(ctx, &atproto.RepoDescribeRepoParams{
		Repo: must(syntax.ParseAtIdentifier("did:plc:ewvi7nxzyoun6zhxrhs64oiz")),
	})
	fmt.Printf("%#v\n", err.(*xrpc.ErrorResponse))
	is.NoErr(err)
	fmt.Printf("%+v\n", res)
}

func TestNewATURI(t *testing.T) {
	is := is.New(t)
	uri := newATURI("did:web:example.com", "com.example.Profile", "")
	is.Equal(syntax.ATURI("at://did:web:example.com/com.example.Profile"), uri)
	uri = newATURI("did:web:example.com", "com.example.Profile", "self")
	is.Equal(syntax.ATURI("at://did:web:example.com/com.example.Profile/self"), uri)
}
