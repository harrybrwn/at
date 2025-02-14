package pds

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/pkg/errors"

	atpapi "github.com/harrybrwn/at/api/com/atproto"
	"github.com/harrybrwn/at/internal/account"
	"github.com/harrybrwn/at/internal/accountstore"
	"github.com/harrybrwn/at/internal/actorstore"
	"github.com/harrybrwn/at/internal/auth"
	"github.com/harrybrwn/at/internal/cid"
	"github.com/harrybrwn/at/internal/parallel"
	"github.com/harrybrwn/at/internal/repo"
	"github.com/harrybrwn/at/internal/sequencer"
	"github.com/harrybrwn/at/xrpc"
)

func (pds *PDS) CreateInviteCode(ctx context.Context, req *atpapi.ServerCreateInviteCodeRequest) (*atpapi.ServerCreateInviteCodeResponse, error) {
	code := genInviteCode(pds.cfg)
	forAccount := req.ForAccount.String()
	if len(forAccount) == 0 {
		forAccount = "admin"
	}
	err := pds.Accounts.CreateInviteCode(ctx, []string{code}, forAccount, int(req.UseCount))
	if err != nil {
		return nil, err
	}
	return &atpapi.ServerCreateInviteCodeResponse{Code: code}, nil
}

func (pds *PDS) DescribeServer(ctx context.Context) (*atpapi.ServerDescribeServerResponse, error) {
	var err error
	res := atpapi.ServerDescribeServerResponse{
		InviteCodeRequired: pds.cfg.Invite.Required,
		AvailableUserDomains: []string{
			"." + pds.cfg.Hostname,
		},
	}
	res.DID, err = syntax.ParseDID(fmt.Sprintf("did:web:%s", pds.cfg.Hostname))
	if err != nil {
		return nil, xrpc.NewInternalError("Invalid configuration")
	}
	res.Links.PrivacyPolicy = pds.cfg.PrivacyPolicyURL
	res.Links.TermsOfService = pds.cfg.TermsOfServiceURL
	res.Contact.Email = pds.cfg.ContactEmailAddress
	return &res, nil
}

func (pds *PDS) CreateAccount(
	ctx context.Context,
	req *atpapi.ServerCreateAccountRequest,
) (*atpapi.ServerCreateAccountResponse, error) {
	inputs, err := validateCreateAccountReqLocalPDS(ctx, pds, req)
	if err != nil {
		pds.logger.Warn("request validation failed", "error", err)
		return nil, err
	}
	did := inputs.did
	rr, err := pds.ActorStore.CreateAsRepo(did, inputs.signingKey)
	if err != nil {
		pds.logger.Warn("failed to create repo", "error", err)
		return nil, err
	}
	defer rr.Close()
	var commit *repo.CommitData
	err = rr.Tx(ctx, pds.newBlobstore(did), func(ctx context.Context, tx *actorstore.RepoTransactor) error {
		var err error
		commit, err = tx.CreateRepo(ctx, nil)
		return err
	})
	if err != nil {
		pds.logger.Warn("repo transactor failed", "error", err)
		return nil, err
	}
	accessJwt, refreshJwt, err := pds.Accounts.CreateAccount(ctx, accountstore.CreateAccountOpts{
		DID:         did.String(),
		Handle:      inputs.handle.String(),
		Email:       &req.Email,
		Password:    &req.Password,
		Deactivated: &inputs.deactivated,
	})
	if err != nil {
		pds.logger.Warn("account managager failed to create account", "error", err)
		return nil, err
	}

	if !inputs.deactivated {
		now := time.Now().Format(time.RFC3339)
		pub, err := pds.Bus.Publisher(ctx)
		if err != nil {
			return nil, err
		}
		defer pub.Close()
		err = parallel.Do(ctx,
			func(ctx context.Context) error {
				return pub.Pub(ctx, sequencer.NewEvent(&Event{SyncSubscribeReposIdentity: &atpapi.SyncSubscribeReposIdentity{
					DID:    did,
					Handle: inputs.handle,
					Time:   now,
				}}))
			},
			func(ctx context.Context) error {
				_ = commit
				// TODO commit events are more complicated than this
				return pub.Pub(ctx, sequencer.NewEvent(&Event{SyncSubscribeReposCommit: &atpapi.SyncSubscribeReposCommit{
					Repo:   did,
					Commit: cid.Cid(commit.CID),
					Prev:   cid.Cid(commit.Prev),
					Rev:    commit.Rev,
					Since:  commit.Since,
					Blobs:  make([]cid.Cid, 0),
					Blocks: []byte{},
					Time:   now,
				}}))
			},
			func(ctx context.Context) error {
				return pub.Pub(ctx, sequencer.NewEvent(&Event{SyncSubscribeReposAccount: &atpapi.SyncSubscribeReposAccount{
					DID:    did,
					Active: true,
					Status: account.StatusActive.String(),
				}}))
			})
		if err != nil {
			return nil, err
		}
	}

	return &atpapi.ServerCreateAccountResponse{
		AccessJwt:  accessJwt,
		RefreshJwt: refreshJwt,
		DID:        inputs.did,
		Handle:     inputs.handle,
		DidDoc:     nil,
	}, nil
}

func (pds *PDS) CreateSession(
	ctx context.Context,
	req *atpapi.ServerCreateSessionRequest,
) (*atpapi.ServerCreateSessionResponse, error) {
	login, err := pds.Accounts.Login(ctx, req.Identifier, req.Password)
	if err != nil {
		return nil, err
	}
	status, active := formatAccountStatus(login.User)
	res := atpapi.ServerCreateSessionResponse{
		Active: active,
		Status: status.String(),
	}

	err = parallel.Do(ctx,
		func(ctx context.Context) (err error) {
			res.AccessJwt, res.RefreshJwt, err = pds.Accounts.CreateSession(
				ctx,
				login.User.DID,
				login.AppPassword,
			)
			return err
		},
		func(ctx context.Context) (err error) {
			if login.User == nil {
				res.Handle = syntax.HandleInvalid
				return nil
			}
			res.DidDoc, err = pds.Resolver.GetDocument(ctx, login.User.DID)
			if err != nil {
				return errors.Wrap(err, "failed to get did document")
			}
			res.DID, err = syntax.ParseDID(login.User.DID)
			if err != nil {
				err = errors.WithStack(err)
				return xrpc.NewInternalError("invalid did syntax").Wrap(err)
			}
			if login.User.Handle.Valid {
				res.Handle, err = syntax.ParseHandle(login.User.Handle.String)
				if err != nil {
					return xrpc.NewInternalError("invalid handle syntax").
						Wrap(errors.WithStack(err))
				}
			} else {
				res.Handle = syntax.HandleInvalid
			}
			res.Email = login.User.Email
			res.EmailConfirmed = login.User.EmailConfirmedAt.Valid
			return nil
		})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (pds *PDS) RefreshSession(ctx context.Context) (*atpapi.ServerRefreshSessionResponse, error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, xrpc.NewAuthRequired("Malformed token")
	}
	acct, err := pds.Accounts.GetAccount(ctx, user.DID, new(accountstore.GetAccountOpts).
		WithDeactivated().
		WithTakenDown())
	if err != nil {
		return nil, err
	}
	if acct.TakedownRef.Valid {
		return nil, &xrpc.ErrorResponse{
			Code:    "AccountTakedown",
			Message: "Account has been taken down.",
		}
	}
	err = parallel.Do(ctx,
		func(ctx context.Context) error {
			return nil
		},
		func(ctx context.Context) error {
			return nil
		})
	if err != nil {
		return nil, err
	}
	return nil, xrpc.ErrNotImplemented
}

type createAccountValidatedInputs struct {
	handle      syntax.Handle
	did         syntax.DID
	signingKey  *crypto.PrivateKeyK256
	plcOp       string
	deactivated bool
}

func validateCreateAccountReqLocalPDS(
	ctx context.Context,
	pds *PDS,
	req *atpapi.ServerCreateAccountRequest,
) (res *createAccountValidatedInputs, err error) {
	requester := auth.UserFromContext(ctx)
	var inviteRequired bool
	if pds.cfg.Invite != nil && pds.cfg.Invite.Required {
		inviteRequired = true
		if len(req.InviteCode) == 0 {
			return nil, &xrpc.ErrorResponse{
				Code:    "InvalidInviteCode",
				Message: "No invite code provided",
			}
		}
	}
	// TODO if email not valid or email is "disposable" throw "This email address is not supported"
	if len(req.Email) == 0 {
		return nil, xrpc.NewInvalidRequest("Email is requred")
	}
	if len(req.Password) == 0 {
		return nil, xrpc.NewInvalidRequest("Password is required")
	}
	handle, err := normalizeAndValidateHandle(ctx, pds, req.Handle, req.DID, false)
	if err != nil {
		return nil, err
	}
	if inviteRequired && len(req.InviteCode) > 0 {
		err = pds.Accounts.EnsureInviteAvailable(ctx, req.InviteCode)
		if err != nil {
			return nil, err
		}
	}

	err = parallel.Do(ctx,
		func(ctx context.Context) error {
			_, err = pds.Accounts.GetAccount(ctx, handle.String(), nil)
			if err == nil {
				return xrpc.NewInvalidRequest("Handle already taken: %s", handle)
			}
			return nil
		},
		func(ctx context.Context) error {
			_, err = pds.Accounts.GetAccountByEmail(ctx, req.Email, nil)
			if err == nil {
				return xrpc.NewInvalidRequest("Email already taken: %s", req.Email)
			}
			return nil
		})
	if err != nil {
		return nil, err
	}

	signingKey, err := crypto.GeneratePrivateKeyK256()
	if err != nil {
		return nil, xrpc.NewInternalError("Account creation failed").Wrap(err)
	}

	var (
		plcOp       string
		did         syntax.DID
		deactivated = false
	)
	if len(req.DID) > 0 {
		if requester == nil || requester.DID != req.DID.String() {
			return nil, xrpc.NewAuthRequired("Missing auth to create account with did: %s", req.DID)
		}
		did = req.DID
		plcOp = ""
		deactivated = true
	} else {
		// did, plcOp, err = formatDidAndPlcOp(ctx, pds, handle, req, signingKey)
		// if err != nil {
		// 	return nil, err
		// }
		newDID, err := pds.PLC.CreateDID(
			ctx,
			nil,
			req.RecoveryKey,
			handle.String(),
			pds.cfg.PublicURL(),
		)
		if err != nil {
			return nil, xrpc.NewInternalError("Failed to create did").Wrap(err)
		}
		did, err = syntax.ParseDID(newDID)
		if err != nil {
			return nil, xrpc.NewInternalError("plc returned invalid did").Wrap(err)
		}
	}
	pds.logger.Debug("created new did", "did", did)
	return &createAccountValidatedInputs{
		handle:      handle,
		did:         did,
		signingKey:  signingKey,
		plcOp:       plcOp,
		deactivated: deactivated,
	}, nil
}

func formatAccountStatus(a *account.ActorAccount) (account.Status, bool) {
	if a == nil {
		return account.StatusDeleted, false
	}
	if a.TakedownRef.Valid {
		return account.StatusTakendown, false
	}
	if a.DeactivatedAt.Valid {
		return account.StatusDeactivated, false
	}
	return account.StatusNone, true
}

func normalizeAndValidateHandle(ctx context.Context, pds *PDS, h syntax.Handle, did syntax.DID, allowReserved bool) (syntax.Handle, error) {
	h = h.Normalize()
	if !h.AllowedTLD() {
		return h, atpapi.ErrServerCreateAccountInvalidHandle.WithMsg("Handle TLD not allowed")
	}
	if hasExplicitSlur(h.String()) {
		return h, atpapi.ErrServerCreateAccountInvalidHandle.WithMsg("Inappropriate language in handle")
	}

	if isServiceDomain(h.String(), pds.cfg.ServiceHandleDomains) {
		err := ensureHandleServiceConstraints(
			h,
			pds.cfg.ServiceHandleDomains,
			allowReserved,
		)
		if err != nil {
			return h, err
		}
	} else {
		if len(did) == 0 {
			return h, atpapi.ErrServerCreateAccountUnsupportedDomain.WithMsg("Not a supported handle domain")
		}
		resolved, err := pds.Resolver.ResolveHandle(ctx, h.String())
		if err != nil {
			return h, xrpc.NewInvalidRequest("Failed to resolved handle %q", h).Wrap(err)
		}
		if resolved != did {
			return h, xrpc.NewInvalidRequest("External handle did not resolve to DID")
		}
	}
	return h, nil
}

func hasExplicitSlur(string) bool {
	return false
}

func isServiceDomain(d string, serviceHandleDomains []string) bool {
	for _, domain := range serviceHandleDomains {
		if strings.HasSuffix(d, domain) {
			return true
		}
	}
	return false
}

// TODO replicate the list in atproto/packages/pds/src/handle/reserved.ts
var reservedDomains = map[string]struct{}{}

func ensureHandleServiceConstraints(h syntax.Handle, availableUserDomains []string, allowReserved bool) error {
	var supportedDomain string
	for _, domain := range availableUserDomains {
		if strings.HasSuffix(h.String(), domain) {
			supportedDomain = domain
			break
		}
	}
	front := string(h[0 : len(h)-len(supportedDomain)])
	if strings.ContainsRune(front, '.') {
		return atpapi.ErrServerCreateAccountInvalidHandle.WithMsg("Invalid characters in handle")
	}
	if len(front) < 3 {
		return atpapi.ErrServerCreateAccountInvalidHandle.WithMsg("Handle too short")
	}
	if len(front) > 18 {
		return atpapi.ErrServerCreateAccountInvalidHandle.WithMsg("Handle too long")
	}
	if !allowReserved {
		if _, ok := reservedDomains[front]; ok {
			return atpapi.ErrServerCreateAccountHandleNotAvailable.WithMsg("Reserved handle")
		}
	}
	return nil
}

func formatDidAndPlcOp(
	ctx context.Context,
	pds *PDS,
	handle syntax.Handle,
	req *atpapi.ServerCreateAccountRequest,
	signingKey *crypto.PrivateKeyK256,
) (syntax.DID, string, error) {
	// pds.plcRotationKey.PublicKey()
	pubPlcRotKey, err := pds.plcRotationKey.PublicKey()
	if err != nil {
		return "", "", err
	}
	rotationDids := []string{pubPlcRotKey.DIDKey()}
	_ = rotationDids
	// did.PubKeyFromCrypto()
	// pds.PLC.CreateDID(ctx, signingKey, req.RecoveryKey, handle.String(), pds.cfg.PublicURL())
	return "", "", nil
}
