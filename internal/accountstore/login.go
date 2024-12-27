package accountstore

import (
	"context"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/harrybrwn/at/internal/account"
	"github.com/harrybrwn/at/xrpc"
)

type Login struct {
	User        *account.ActorAccount
	AppPassword *AppPassDescript
}

func (as *AccountStore) Login(ctx context.Context, identifier, password string) (login *Login, err error) {
	start := time.Now()
	defer func() {
		dur := int64(time.Since(start))
		n := rand.Int64N(int64(time.Millisecond*350)) - dur
		time.Sleep(time.Duration(max(0, n)))
	}()
	var (
		user *account.ActorAccount
		opts = new(GetAccountOpts).
			WithTakenDown().
			WithDeactivated()
	)
	identifier = strings.ToLower(identifier)
	if strings.Contains(identifier, "@") {
		user, err = as.GetAccountByEmail(ctx, identifier, opts)
	} else {
		user, err = as.GetAccount(ctx, identifier, opts)
	}
	if err != nil {
		err = xrpc.NewAuthRequired("Invalid identifier or password").Wrap(err)
		return
	}

	var appPassword *AppPassDescript
	err = as.VerifyAccountPassword(ctx, user.DID, password)
	if err != nil {
		var apErr error
		appPassword, apErr = as.VerifyAppPassword(ctx, user.DID, password)
		if apErr != nil {
			return nil, xrpc.NewAuthRequired("Invalid identifier or password").
				Wrap(errors.Wrap(apErr, "failed to verify app password"))
		}
	}
	if softDeleted(user) {
		err = &xrpc.ErrorResponse{
			Code:    "AccountTakedown",
			Message: "Account has been taken down",
		}
		return nil, err
	}
	return &Login{User: user, AppPassword: appPassword}, nil
}

func softDeleted(user *account.ActorAccount) bool {
	// If the takedownRef is valid (meaning "not null") then the account has
	// been soft deleted.
	return user.TakedownRef.Valid
}
