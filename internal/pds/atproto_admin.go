package pds

import (
	"context"

	"github.com/harrybrwn/at/api/com/atproto"
	"github.com/harrybrwn/at/xrpc"
)

type AtprotoAdmin interface {
	atproto.AdminDeleteAccount
	atproto.AdminDisableAccountInvites
	atproto.AdminDisableInviteCodes
	atproto.AdminEnableAccountInvites
	atproto.AdminGetAccountInfo
	atproto.AdminGetAccountInfos
	atproto.AdminGetSubjectStatus
	atproto.AdminSearchAccounts
	atproto.AdminUpdateAccountEmail
	atproto.AdminUpdateAccountHandle
	atproto.AdminUpdateAccountPassword
	atproto.AdminUpdateSubjectStatus
}

var _ AtprotoAdmin = (*PDS)(nil)

func (pds *PDS) DeleteAccount(ctx context.Context, req *atproto.AdminDeleteAccountRequest) (any, error) {
	err := pds.ActorStore.Destroy(req.DID)
	if err != nil {
		return nil, err
	}
	err = pds.Accounts.DeleteAccount(ctx, req.DID.String())
	if err != nil {
		return nil, err
	}
	// TODO sequencers
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) DisableAccountInvites(ctx context.Context, req *atproto.AdminDisableAccountInvitesRequest) (any, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) EnableAccountInvites(ctx context.Context, req *atproto.AdminEnableAccountInvitesRequest) (any, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) DisableInviteCodes(ctx context.Context, req *atproto.AdminDisableInviteCodesRequest) (any, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) GetAccountInfo(ctx context.Context, req *atproto.AdminGetAccountInfoParams) (*atproto.AdminGetAccountInfoResponse, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) GetAccountInfos(ctx context.Context, req *atproto.AdminGetAccountInfosParams) (*atproto.AdminGetAccountInfosResponse, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) GetInviteCodes(ctx context.Context, req *atproto.AdminGetInviteCodesParams) (*atproto.AdminGetInviteCodesResponse, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) GetSubjectStatus(ctx context.Context, req *atproto.AdminGetSubjectStatusParams) (*atproto.AdminGetSubjectStatusResponse, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) SearchAccounts(ctx context.Context, req *atproto.AdminSearchAccountsParams) (*atproto.AdminSearchAccountsResponse, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) SendEmail(ctx context.Context, req *atproto.AdminSendEmailRequest) (*atproto.AdminSendEmailResponse, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) UpdateAccountEmail(ctx context.Context, req *atproto.AdminUpdateAccountEmailRequest) (any, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) UpdateAccountHandle(ctx context.Context, req *atproto.AdminUpdateAccountHandleRequest) (any, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) UpdateAccountPassword(ctx context.Context, req *atproto.AdminUpdateAccountPasswordRequest) (any, error) {
	return nil, xrpc.ErrNotImplemented
}

func (pds *PDS) UpdateSubjectStatus(ctx context.Context, req *atproto.AdminUpdateSubjectStatusRequest) (*atproto.AdminUpdateSubjectStatusResponse, error) {
	return nil, xrpc.ErrNotImplemented
}
