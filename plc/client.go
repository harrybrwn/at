package plc

import (
	"context"

	"github.com/whyrusleeping/go-did"
)

type Client interface {
	CreateDID(ctx context.Context, signingkey *did.PrivKey, recovery, handle string) (string, error)
	UpdateUserHandle(ctx context.Context, didstr, nhandle string) error
	DeactivateDID(ctx context.Context, did string) error
}
