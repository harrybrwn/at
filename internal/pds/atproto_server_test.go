package pds

import (
	"strings"
	"testing"

	"github.com/matryer/is"

	"github.com/harrybrwn/at/api/com/atproto"
	"github.com/harrybrwn/at/internal/auth"
	"github.com/harrybrwn/at/internal/cbor/dagcbor"
	"github.com/harrybrwn/at/xrpc"
)

func TestCreateInviteCode(t *testing.T) {
	is := is.New(t)
	pds := testPDS(t, localhost)
	ctx := t.Context()
	res, err := pds.CreateInviteCode(ctx, &atproto.ServerCreateInviteCodeRequest{
		UseCount: 1,
	})
	is.NoErr(err)
	is.True(len(res.Code) > 0)
}

func TestCreateAccount(t *testing.T) {
	is := is.New(t)
	pds := testPDS(t, localhost)
	ctx := t.Context()
	invite, err := pds.CreateInviteCode(ctx, &atproto.ServerCreateInviteCodeRequest{UseCount: 1})
	is.NoErr(err)
	res, err := pds.CreateAccount(ctx, &atproto.ServerCreateAccountRequest{
		Email:      "me@test.local",
		Handle:     "new-user.test",
		Password:   "testlab01",
		InviteCode: invite.Code,
	})
	is.NoErr(err)
	is.Equal(res.Handle.String(), "new-user.test")
	is.True(len(res.DID) > 0)
	is.True(strings.HasPrefix(res.DID.String(), "did:plc:"))
	is.True(len(res.AccessJwt) > 0)
	is.True(len(res.RefreshJwt) > 0)
}

func TestCreateAccount_WithDID(t *testing.T) {
	is := is.New(t)
	pds := testPDS(t, localhost)
	ctx := t.Context()
	ctx = auth.StashUser(ctx, &xrpc.Auth{
		DID:    "did:plc:nsu4iq7726acidyqpha2zuk3",
		Handle: "harry.test",
	})
	invite, err := pds.CreateInviteCode(ctx, &atproto.ServerCreateInviteCodeRequest{UseCount: 1})
	is.NoErr(err)
	res, err := pds.CreateAccount(ctx, &atproto.ServerCreateAccountRequest{
		DID:        "did:plc:nsu4iq7726acidyqpha2zuk3", // This did has actual entry in plc.directory
		Handle:     "harry.test",
		Email:      "me@test.local",
		Password:   "nVfokUEyeq3q0qQ5cprPeiLh",
		InviteCode: invite.Code,
	})
	is.NoErr(err)
	is.Equal(res.DID.String(), "did:plc:nsu4iq7726acidyqpha2zuk3")
	is.Equal(res.Handle.String(), "harry.test")
	is.True(len(res.AccessJwt) > 0)
	is.True(len(res.RefreshJwt) > 0)
}

func TestCreateSession(t *testing.T) {
	is := is.New(t)
	pds := testPDS(t, localhost)
	ctx := t.Context()
	eventCount := 0
	go func() {
		sub, err := pds.Bus.Subscriber(ctx)
		if err != nil {
			panic(err)
		}
		defer sub.Close()
		events, _ := sub.Sub(ctx)
		for e := range events {
			eventCount++
			b, err := dagcbor.Marshal(e.Event)
			if err != nil {
				panic(err)
			}
			_ = b
		}
	}()
	invite, err := pds.CreateInviteCode(ctx, &atproto.ServerCreateInviteCodeRequest{UseCount: 1})
	is.NoErr(err)
	acct, err := pds.CreateAccount(ctx, &atproto.ServerCreateAccountRequest{
		Email:      "me@test.local",
		Handle:     "new-user.test",
		Password:   "testlab01",
		InviteCode: invite.Code,
	})
	is.NoErr(err)
	is.Equal(eventCount, 3)
	session, err := pds.CreateSession(ctx, &atproto.ServerCreateSessionRequest{
		Identifier: "new-user.test",
		Password:   "testlab01",
	})
	is.NoErr(err)
	is.True(len(session.AccessJwt) > 0)
	is.True(len(session.RefreshJwt) > 0)
	is.True(len(session.DID) > 0)
	is.True(session.Active)
	is.Equal(acct.DID, session.DID)
	is.Equal(session.Email, "me@test.local")
}
