package pds

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/pkg/errors"

	atpapi "github.com/harrybrwn/at/api/com/atproto"
	"github.com/harrybrwn/at/internal/accountstore"
	"github.com/harrybrwn/at/internal/actorstore"
	"github.com/harrybrwn/at/internal/auth"
	"github.com/harrybrwn/at/internal/repo"
	"github.com/harrybrwn/at/xrpc"
)

func (pds *PDS) DescribeRepo(ctx context.Context, req *atpapi.RepoDescribeRepoParams) (*atpapi.RepoDescribeRepoResponse, error) {
	var (
		err           error
		did           syntax.DID
		handleCorrect bool
		handle        = syntax.HandleInvalid
	)
	if req.Repo.IsHandle() {
		handle, err = req.Repo.AsHandle()
		if err != nil {
			return nil, xrpc.Wrap(err, xrpc.InvalidRequest, "invalid handle")
		}
		did, err = pds.Resolver.ResolveHandle(ctx, handle.String())
		if err != nil {
			return nil, xrpc.Wrapf(err, xrpc.InternalServerError, "failed to resolve handle %q", handle)
		}
	} else {
		did, err = req.Repo.AsDID()
		if err != nil {
			return nil, xrpc.Wrap(err, xrpc.InvalidRequest, "invalid did")
		}
	}

	account, err := pds.Accounts.GetAccount(ctx, req.Repo.String(), &accountstore.GetAccountOpts{
		IncludeTakenDown:   true,
		IncludeDeactivated: true,
	})
	if err != nil {
		return nil, xrpc.Wrapf(err, xrpc.RepoNotFound, "Could not find repo for DID: %s", did)
	}
	if account.TakedownRef.Valid {
		return nil, &xrpc.ErrorResponse{
			Code:    "RepoTakendown",
			Message: fmt.Sprintf("Repo has been takendown: %s", did),
		}
	}
	if account.DeactivatedAt.Valid {
		return nil, &xrpc.ErrorResponse{
			Code:    "RepoDeactivated",
			Message: fmt.Sprintf("Repo has been deactivated: %s", did),
		}
	}
	doc, err := pds.Resolver.GetDocument(ctx, did.String())
	if err != nil {
		return nil, xrpc.Status(http.StatusNotFound, "couldn't resolve did").Wrap(err)
	}
	handle = getHandle(doc.AlsoKnownAs)
	handleCorrect = handle.String() == account.Handle.String
	rr, err := pds.ActorStore.Record(did)
	if err != nil {
		return nil, xrpc.Wrap(
			errors.Wrapf(err, "Could not find actor for DID: %q", did),
			xrpc.InternalServerError,
			"internal error",
		)
	}

	collectionSeq, err := rr.ListCollections(ctx)
	if err != nil {
		return nil, xrpc.Wrap(err, xrpc.InvalidRequest, "Could not find collections")
	}
	collections, err := gatherCollections(collectionSeq)
	if err != nil {
		return nil, xrpc.Wrap(err, xrpc.InternalServerError, "Could not find collections")
	}
	return &atpapi.RepoDescribeRepoResponse{
		DID:             did,
		Handle:          handle,
		DidDoc:          doc,
		Collections:     collections,
		HandleIsCorrect: handleCorrect,
	}, nil
}

func (pds *PDS) GetRecord(ctx context.Context, r *atpapi.RepoGetRecordParams) (*atpapi.RepoGetRecordResponse, error) {
	if !r.Repo.IsDID() {
		return atpapi.NewRepoClient(pds.Passthrough).GetRecord(ctx, r)
	}
	did, err := r.Repo.AsDID()
	if err != nil {
		return nil, err
	}
	// TODO check to see if we have the DID in our database
	rr, err := pds.ActorStore.Record(did)
	if err != nil {
		return nil, err
	}
	defer rr.Close()

	uri := fmt.Sprintf("at://%s/%s/%s", did, r.Collection, r.RKey)
	res, err := rr.GetRecord(ctx, syntax.ATURI(uri), r.CID, false)
	if err != nil {
		return nil, xrpc.Wrap(err, xrpc.RecordNotFound, "couldn't find record")
	}
	if res.TakedownRef.Valid {
		return nil, errors.New("record was taken down")
	}
	return &atpapi.RepoGetRecordResponse{
		URI:   res.URI,
		CID:   res.CID,
		Value: res.Value,
	}, nil
}

func (pds *PDS) PutRecord(ctx context.Context, r *atpapi.RepoPutRecordRequest) (*atpapi.RepoPutRecordResponse, error) {
	auth := auth.UserFromContext(ctx)
	if auth == nil {
		return nil, xrpc.NewAuthRequired("Auth required")
	}
	acct, err := pds.Accounts.GetAccount(ctx, r.Repo.String(), new(accountstore.GetAccountOpts).WithDeactivated())
	if err != nil {
		return nil, err
	}
	if acct.DeactivatedAt.Valid {
		return nil, xrpc.NewInvalidRequest("Account is deactivated")
	} else if auth.DID != acct.DID {
		return nil, xrpc.NewAuthRequired("Wrong user")
	}
	did, err := syntax.ParseDID(acct.DID)
	if err != nil {
		return nil, xrpc.NewInternalError("invalid state").Wrap(err)
	}
	collection := r.Collection
	record := r.Record
	rkey := r.RKey
	recordCid, err := repo.NewCID(record)
	if err != nil {
		return nil, xrpc.NewInternalError("Failed to hash record").Wrap(err)
	}
	err = setCollectionName(record, collection)
	if err != nil {
		return nil, err
	}
	uri := newATURI(did, collection, rkey)

	var (
		write  repo.PreparedWrite
		commit *repo.CommitData
	)
	err = pds.ActorStore.Transact(ctx, did, pds.newBlobstore(did), func(ctx context.Context, tx *actorstore.ActorStoreTransactor) error {
		current, err := tx.Record.GetRecord(ctx, uri, nil, false)
		if err != nil {
			return err
		}
		isUpdate := current != nil
		if isUpdate && r.Collection == "app.bsky.actor.profile" {
			// TODO updateProfileLegacyBlobRef(actorTxn, record)
		}
		if isUpdate {
			update := repo.PreparedUpdate{
				Action: repo.WriteOpActionUpdate,
				URI:    uri,
				CID:    recordCid,
				Record: record,
			}
			// TODO write assertNoExplicitSlurs(rkey, record)
			if r.SwapCommit.ByteLen() > 0 {
				update.SwapCID = &r.SwapCommit
			}
			// TODO Set update.ValidationStatus by getting the lexicon
			// definition dynamically and validating the record.
			update.ValidationStatus = repo.ValidationStatusValid
			write.PreparedUpdate = &update
		} else {
			create := repo.PreparedCreate{
				Action: repo.WriteOpActionCreate,
				URI:    uri,
				CID:    recordCid,
				Record: record,
			}
			if len(rkey) == 0 {
				rkey = repo.NextTID().String()
			}
			_, err = syntax.ParseRecordKey(rkey)
			if err != nil {
				return xrpc.NewInvalidRequest("Invalid rkey: %q: %v", rkey, err.Error()).Wrap(err)
			}
			// TODO write assertNoExplicitSlurs(rkey, record)
			if r.SwapCommit.ByteLen() > 0 {
				create.SwapCID = &r.SwapCommit
			}
			// TODO Set create.ValidationStatus by getting the lexicon
			// definition dynamically and validating the record.
			create.ValidationStatus = repo.ValidationStatusValid
			write.PreparedCreate = &create
		}

		// no-op
		if current != nil && current.CID.String() == recordCid.String() {
			commit = nil
			return nil
		}
		commit, err = tx.Repo.ProcessWrites(ctx, []repo.PreparedWrite{write}, r.SwapCommit)
		return err
	})
	if err != nil {
		return nil, err
	}
	if commit != nil {
		// TODO emit commit events
	}
	return &atpapi.RepoPutRecordResponse{
		URI: write.GetURI(),
		CID: write.GetCID(),
	}, nil
}

func (pds *PDS) ListRecords(ctx context.Context, r *atpapi.RepoListRecordsParams) (*atpapi.RepoListRecordsResponse, error) {
	if !r.Repo.IsDID() {
		return atpapi.NewRepoClient(pds.Passthrough).ListRecords(ctx, r)
	}
	did, err := r.Repo.AsDID()
	if err != nil {
		return nil, err
	}
	// TODO check to see if we have the DID in our database
	rr, err := pds.ActorStore.Record(did)
	if err != nil {
		return nil, xrpc.Wrap(
			errors.Wrapf(err, "could not find open record reader for %q", did),
			xrpc.InternalServerError,
			"internal error",
		)
	}
	res, err := rr.ListForCollection(ctx, r)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (pds *PDS) CreateRecord(ctx context.Context, req *atpapi.RepoCreateRecordRequest) (*atpapi.RepoCreateRecordResponse, error) {
	account, err := pds.Accounts.GetAccount(ctx, req.Repo.String(), &accountstore.GetAccountOpts{
		IncludeDeactivated: true,
	})
	if err != nil {
		return nil, xrpc.NewInvalidRequest("Could not find repo: %s", req.Repo)
	} else if account.DeactivatedAt.Valid {
		return nil, xrpc.NewInvalidRequest("Account is deactivated")
	}
	did := account.DID
	auth := auth.UserFromContext(ctx)
	if auth == nil || did != auth.DID {
		return nil, &xrpc.ErrorResponse{Code: xrpc.AuthRequired}
	}
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) DeleteRecord(cxt context.Context, req *atpapi.RepoDeleteRecordRequest) (*atpapi.RepoDeleteRecordResponse, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) UploadBlob(ctx context.Context, body io.Reader) (*atpapi.RepoUploadBlobResponse, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) ApplyWrites(ctx context.Context, req *atpapi.RepoApplyWritesRequest) (*atpapi.RepoApplyWritesResponse, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) ListMissingBlobs(ctx context.Context, params *atpapi.RepoListMissingBlobsParams) (*atpapi.RepoListMissingBlobsResponse, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) ImportRepo(ctx context.Context, body io.Reader) (any, error) {
	return nil, xrpc.ErrNotImplemented
}

func setCollectionName(record any, collection syntax.NSID) error {
	if m, ok := record.(map[string]any); ok {
		if typ, ok := m["$type"]; ok {
			if typ.(string) != collection.String() {
				return xrpc.NewInvalidRequest("Invalid $type: expected %q, got %q", collection, typ)
			}
		} else {
			m["$type"] = collection.String()
		}
	}
	return nil
}

func newATURI[R, C, K ~string](repo R, collection C, rkey K) syntax.ATURI {
	u := url.URL{
		Scheme: "at",
		Path:   path.Join(string(repo), string(collection), string(rkey)),
	}
	return syntax.ATURI(u.String())
}
