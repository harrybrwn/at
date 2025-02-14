package auth

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"time"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
)

type Scope string

const (
	ScopeAccess            Scope = "com.atproto.access"
	ScopeRefresh           Scope = "com.atproto.refresh"
	ScopeAppPass           Scope = "com.atproto.appPass"
	ScopeAppPassPrivileged Scope = "com.atproto.appPassPrivileged"
	ScopeSignupQueued      Scope = "com.atproto.signupQueued"
)

type CreateTokenOpts struct {
	DID        string
	JWTKey     []byte
	ServiceDID string
	Scope      Scope
	ExpiresIn  time.Duration
	JTI        string // only used for creating refresh tokens
	Now        *time.Time
}

func CreateTokens(opts *CreateTokenOpts) (access, refresh string, err error) {
	opts.Scope = ScopeAccess
	access, err = CreateAccessToken(opts)
	if err != nil {
		return
	}
	opts.Scope = ScopeRefresh
	refresh, err = CreateRefreshToken(opts)
	if err != nil {
		return
	}
	return access, refresh, nil
}

// CreateAccessToken generates an access token.
func CreateAccessToken(opts *CreateTokenOpts) (string, error) {
	if opts.Scope == "" {
		opts.Scope = ScopeAccess
	}
	expiresIn := opts.ExpiresIn
	if expiresIn == 0 {
		expiresIn = time.Hour * 2160 // 90 days
	}

	var now time.Time
	if opts.Now != nil {
		now = *opts.Now
	} else {
		now = time.Now().UTC()
	}
	expirationTime := now.Add(expiresIn)
	claims := jwt.MapClaims{
		"scope": opts.Scope,
		"aud":   opts.ServiceDID,
		"sub":   opts.DID,
		"iat":   now.Unix(),
		"exp":   expirationTime.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(opts.JWTKey)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return signedToken, nil
}

// CreateRefreshToken generates a refresh token.
func CreateRefreshToken(opts *CreateTokenOpts) (string, error) {
	if opts.JTI == "" {
		var err error
		opts.JTI, err = GetRefreshTokenID()
		if err != nil {
			return "", err
		}
	}
	expiresIn := opts.ExpiresIn
	if expiresIn == 0 {
		expiresIn = time.Hour * 2160 // 90 days
	}

	var now time.Time
	if opts.Now != nil {
		now = *opts.Now
	} else {
		now = time.Now().UTC()
	}
	expirationTime := now.Add(expiresIn)
	claims := jwt.MapClaims{
		"scope": ScopeRefresh,
		"aud":   opts.ServiceDID,
		"sub":   opts.DID,
		"jti":   opts.JTI,
		"iat":   now.Unix(),
		"exp":   expirationTime.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(opts.JWTKey)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return signedToken, nil
}

func GenerateJTI() (string, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", errors.New("failed to generate random bytes")
	}
	encoded := base64.StdEncoding.EncodeToString(bytes)
	return encoded[:len(encoded)-1], nil // Trim padding '=' characters
}

// getRefreshTokenID generates a unique identifier for the refresh token.
func GetRefreshTokenID() (string, error) {
	return GenerateJTI()
}

// GenerateRandomToken generates a random token formatted as xxxxx-xxxxx
func GenerateRandomToken() (string, error) {
	// Generate 8 random bytes
	bytes := make([]byte, 8)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate random bytes")
	}
	// Encode to base32 and take first 10 characters
	token := base32.StdEncoding.EncodeToString(bytes)[:10]
	// Format as xxxxx-xxxxx
	return token[:5] + "-" + token[5:10], nil
}

type (
	SigningMethodK256 struct{}
	SigningMethodP256 struct{}
)

var (
	K256SigningMethod *SigningMethodK256
	P256SigningMethod *SigningMethodP256
)

func init() {
	K256SigningMethod = new(SigningMethodK256)
	jwt.RegisterSigningMethod("ES256K", func() jwt.SigningMethod {
		return K256SigningMethod
	})
	P256SigningMethod = new(SigningMethodP256)
	jwt.RegisterSigningMethod("ES256", func() jwt.SigningMethod {
		return P256SigningMethod
	})
}

// Returns nil if signature is valid
func (sm *SigningMethodK256) Verify(signingString string, sig []byte, key any) (err error) {
	var pubkey *crypto.PublicKeyK256
	switch k := key.(type) {
	case *crypto.PublicKeyK256:
		pubkey = k
	case *crypto.PrivateKeyK256:
		var pk crypto.PublicKey
		pk, err = k.PublicKey()
		if err != nil {
			return err
		}
		pubkey = pk.(*crypto.PublicKeyK256)
	case []byte:
		pk, err := crypto.ParsePrivateBytesK256(k)
		if err != nil {
			return err
		}
		var pub crypto.PublicKey
		pub, err = pk.PublicKey()
		if err != nil {
			return err
		}
		pubkey = pub.(*crypto.PublicKeyK256)
	default:
		return errors.Errorf("wrong key type: got %T", key)
	}
	err = pubkey.HashAndVerify([]byte(signingString), sig)
	if errors.Is(err, crypto.ErrInvalidSignature) {
		return jwt.ErrSignatureInvalid
	}
	return err
}

// Returns signature or error
func (sm *SigningMethodK256) Sign(signingString string, key any) ([]byte, error) {
	switch k := key.(type) {
	case *crypto.PrivateKeyK256:
		return k.HashAndSign([]byte(signingString))
	case []byte:
		pk, err := crypto.ParsePrivateBytesK256(k)
		if err != nil {
			return nil, err
		}
		return pk.HashAndSign([]byte(signingString))
	default:
		return nil, errors.Errorf("wrong key type: got %T", key)
	}
}

func (sm *SigningMethodK256) Alg() string {
	return "ES256K"
}

func (sm *SigningMethodP256) Verify(signingString string, sig []byte, key any) (err error) {
	var pubkey *crypto.PublicKeyP256
	switch k := key.(type) {
	case *crypto.PrivateKeyP256:
		var pk crypto.PublicKey
		pk, err = k.PublicKey()
		if err != nil {
			return err
		}
		pubkey = pk.(*crypto.PublicKeyP256)
	case *crypto.PublicKeyP256:
		pubkey = k
	default:
		return errors.Errorf("wrong key type: got %T", key)
	}
	err = pubkey.HashAndVerify([]byte(signingString), sig)
	if errors.Is(err, crypto.ErrInvalidSignature) {
		return jwt.ErrSignatureInvalid
	}
	return err
}

func (sm *SigningMethodP256) Sign(signingString string, key any) ([]byte, error) {
	switch k := key.(type) {
	case *crypto.PrivateKeyP256:
		return k.HashAndSign([]byte(signingString))
	case []byte:
		pk, err := crypto.ParsePrivateBytesP256(k)
		if err != nil {
			return nil, err
		}
		return pk.HashAndSign([]byte(signingString))
	default:
		return nil, errors.Errorf("wrong key type: got %T", key)
	}
}

func (sm *SigningMethodP256) Alg() string {
	return "ES256"
}
