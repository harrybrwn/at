package pds

import (
	"database/sql"
	"iter"
	"net/url"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"github.com/harrybrwn/at/internal/accountstore"
	"github.com/harrybrwn/at/internal/auth"
	"github.com/harrybrwn/at/xrpc"
)

func NewAccountStore(c *EnvConfig) (*accountstore.AccountStore, error) {
	db, err := sql.Open("sqlite3", c.AccountDBLocation)
	if err != nil {
		return nil, err
	}
	return accountstore.New(db, []byte(c.JwtSecret), c.Service.DID), nil
}

func genInviteCode(cfg *EnvConfig) string {
	token, err := auth.GenerateRandomToken()
	if err != nil {
		panic(err)
	}
	return strings.ReplaceAll(cfg.Hostname, ".", "-") + "-" + token
}

func getHandle(aka []string) syntax.Handle {
	if len(aka) == 0 {
		return syntax.HandleInvalid
	}
	for _, uri := range aka {
		u, err := url.Parse(uri)
		if err != nil {
			continue
		}
		handle, err := syntax.ParseHandle(u.Host)
		if err != nil {
			continue
		}
		return handle
	}
	return syntax.HandleInvalid
}

func gatherCollections(s iter.Seq2[string, error]) ([]syntax.NSID, error) {
	res := make([]syntax.NSID, 0)
	for item, err := range s {
		if err != nil {
			return nil, xrpc.Wrap(err, xrpc.InternalServerError, "couldn't find collections")
		}
		collection, err := syntax.ParseNSID(item)
		if err != nil {
			return nil, xrpc.Wrapf(err, xrpc.InternalServerError, "stored collection %q is invalid", item)
		}
		res = append(res, collection)
	}
	return res, nil
}
