//go:build functional

package repo

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/golang-jwt/jwt/v5"
	"github.com/harrybrwn/at/api/com/atproto"
	"github.com/harrybrwn/at/atp"
	"github.com/harrybrwn/at/internal/auth"
	"github.com/harrybrwn/at/xrpc"
	"github.com/matryer/is"
)

func TestFunc_LoadRepo(t *testing.T) {
	t.Skip()
	ctx := context.Background()
	is := is.New(t)
	did := "did:plc:nsu4iq7726acidyqpha2zuk3"
	client := xrpc.NewClient(
		xrpc.WithHost("localhost:3000"),
		xrpc.WithAdminPassword("testlab"),
		xrpc.WithInsecure(),
		xrpc.WithJwt(servicejwt(t, "com.atproto.server.createAccount")),
	)
	invite, err := atproto.NewServerClient(client).CreateInviteCode(
		ctx,
		&atproto.ServerCreateInviteCodeRequest{
			ForAccount: must(syntax.ParseDID(did)),
			UseCount:   1,
		})
	is.NoErr(err)
	accountRes, err := atproto.NewServerClient(client).CreateAccount(ctx, &atproto.ServerCreateAccountRequest{
		DID:        must(syntax.ParseDID(did)),
		Handle:     "harry.test",
		Email:      "me@test.local",
		Password:   "nVfokUEyeq3q0qQ5cprPeiLh",
		InviteCode: invite.Code,
	})
	is.NoErr(err)
	_ = accountRes
}

func TestFunc_VerifyServiceJwt(t *testing.T) {
	t.Skip()
	ctx := context.Background()
	rawtoken := servicejwt(t, "")
	resolver := atp.Resolver{
		HandleResolver: must(atp.NewDefaultHandleResolver()),
		PlcURL:         must(url.Parse("https://plc.directory")),
		HttpClient:     http.DefaultClient,
	}
	claims := make(jwt.MapClaims)
	tok, err := jwt.ParseWithClaims(rawtoken, &claims, func(t *jwt.Token) (any, error) {
		iss, err := t.Claims.GetIssuer()
		if err != nil {
			return nil, err
		}
		key, err := auth.GetResolverSigningKey(ctx, &resolver, iss)
		if err != nil {
			return nil, err
		}
		return key, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !tok.Valid {
		t.Fatal("JWT signature failed when verifying with did doc verification method")
	}
}

func servicejwt(t *testing.T, lxm string) string {
	t.Helper()
	// NOTE This DID and Private Key are stored in https://plc.directory
	did := "did:plc:nsu4iq7726acidyqpha2zuk3"
	// rawkey is also stored in "testdata/service-jwt-private-key"
	rawkey := []byte{
		0x54, 0x54, 0x39, 0xeb, 0xd0, 0xf3, 0x5e, 0x29, 0xcc, 0x5e, 0xf1,
		0xf3, 0x5e, 0xa5, 0x8, 0x95, 0x6b, 0xeb, 0xb1, 0x25, 0x91, 0x2d,
		0x35, 0xf0, 0xa3, 0x30, 0x62, 0x8e, 0x3e, 0xe8, 0x92, 0xeb}
	key, err := crypto.ParsePrivateBytesK256(rawkey)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	exp := now.Add(time.Hour)
	token, err := auth.CreateServiceJwt(&auth.ServiceJwtOpts{
		Iss:     did,
		Aud:     did,
		KeyPair: key,
		Iat:     &now,
		Exp:     &exp,
		LXM:     &lxm,
	})
	if err != nil {
		t.Fatal(err)
	}
	return token
}
