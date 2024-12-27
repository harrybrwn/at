package accountstore

import (
	"context"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/indigo/did"
	"github.com/harrybrwn/db"
	"github.com/pkg/errors"
)

func NewResolver(as *AccountStore, host string) *Resolver {
	return &Resolver{
		as:   as,
		host: host,
	}
}

type Resolver struct {
	as   *AccountStore
	host string
}

func (*Resolver) FlushCacheFor(did string) {}

func (r *Resolver) GetDocument(ctx context.Context, didstr string) (*did.Document, error) {
	d, err := did.ParseDID(didstr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rows, err := r.as.db.QueryContext(ctx, `SELECT handle FROM actor WHERE did = ?`, didstr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var handle string
	err = db.ScanOne(rows, &handle)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var service string
	if r.host == "localhost" {
		service = "http://" + r.host
	} else {
		service = "https://" + r.host
	}
	srvDID, err := did.ParseDID("#atproto_pds")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &did.Document{
		Context: []string{
			"https://www.w3.org/ns/did/v1",
			"https://w3id.org/security/multikey/v1",
			"https://w3id.org/security/suites/secp256k1-2019/v1",
		},
		ID:          d,
		AlsoKnownAs: []string{"at://" + handle},
		Service: []did.Service{
			{
				ID:              srvDID,
				Type:            "AtprotoPersonalDataServer",
				ServiceEndpoint: service,
			},
		},
	}, nil
}

func (r *Resolver) ResolveHandle(ctx context.Context, handle string) (syntax.DID, error) {
	rows, err := r.as.db.QueryContext(
		ctx,
		`SELECT did FROM actor WHERE handle = ?`,
		handle,
	)
	if err != nil {
		return "", errors.WithStack(err)
	}
	var did syntax.DID
	err = db.ScanOne(rows, &did)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return did, nil
}
