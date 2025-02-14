package auth

import (
	"time"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/golang-jwt/jwt/v5"
)

type ServiceJwtOpts struct {
	Iss string
	Aud string
	Iat *time.Time
	Exp *time.Time
	// LXM is the lexicon method
	LXM     *string
	KeyPair crypto.PrivateKey
}

func CreateServiceJwt(params *ServiceJwtOpts) (tokenJwt string, err error) {
	var iat, exp time.Time
	if params.Iat != nil {
		iat = *params.Iat
	} else {
		iat = time.Now()
	}
	if params.Exp != nil {
		exp = *params.Exp
	} else {
		exp = iat.Add(time.Minute)
	}
	var lxm string
	if params.LXM != nil {
		lxm = *params.LXM
	}
	jti, err := GenerateJTI()
	if err != nil {
		return "", err
	}
	token := jwt.NewWithClaims(K256SigningMethod, jwt.MapClaims{
		"iat": iat.UTC().Unix(),
		"exp": exp.UTC().Unix(),
		"iss": params.Iss,
		"aud": params.Aud,
		"jti": jti,
		"lxm": lxm,
	})
	signedToken, err := token.SignedString(params.KeyPair)
	if err != nil {
		return "", err
	}
	return signedToken, nil
}
