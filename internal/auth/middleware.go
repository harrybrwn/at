package auth

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	"github.com/bluesky-social/indigo/atproto/crypto"
	indigodid "github.com/bluesky-social/indigo/did"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/whyrusleeping/go-did"

	"github.com/harrybrwn/at/xrpc"
)

type Middelware func(http.Handler) http.Handler

type ContextKey string

const (
	tokenKey ContextKey = "xrpc-token"
	userKey  ContextKey = "xrpc-user"
)

type Opts struct {
	Logger        *slog.Logger
	JWTSecret     []byte
	AdminPassword string
	Resolver      indigodid.Resolver
}

func (ao *Opts) secret(*jwt.Token) (any, error) {
	return ao.JWTSecret, nil
}

// AuthRequired will extract a jwt token and store it in the request context. It
// will fail hard if the token is invalid or not found.
func Required(opts *Opts) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			raw, tt := getRawToken(r)
			switch tt {
			case invalid, empty:
				err := &xrpc.ErrorResponse{Code: xrpc.AuthRequired}
				xrpc.WriteError(opts.Logger, w, err, xrpc.Forbidden)
				return

			case basic:
				opts.Logger.Info("Using basic auth")
				username, password, err := parseBasicAuth(raw)
				if err != nil {
					xrpc.WriteError(opts.Logger, w, err, xrpc.InvalidRequest)
					return
				}
				if username != "admin" || password != opts.AdminPassword {
					if username != "admin" {
						opts.Logger.Warn("incorrect username", "username", username)
					}
					if password != opts.AdminPassword {
						opts.Logger.Warn("incorrect admin password")
					}
					err := xrpc.ErrorResponse{Code: xrpc.Forbidden}
					xrpc.WriteError(opts.Logger, w, &err, "")
					return
				}
				ctx = storeUser(ctx, &xrpc.Auth{Handle: username})

			case bearer:
				tok, did, err := validateBearerToken(raw, ScopeAccess, opts.secret)
				if err != nil {
					xrpc.WriteError(opts.Logger, w, err, "")
					return
				}
				ctx = storeUser(ctx, &xrpc.Auth{
					DID: did,
				})
				ctx = storeToken(ctx, tok)
			}
			r = r.WithContext(ctx)
			h.ServeHTTP(w, r)
		})
	}
}

func AdminOnly(opts *Opts) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			raw, tt := getRawToken(r)
			switch tt {
			case basic:
				username, err := validateBasicAuth(opts, raw)
				if err != nil {
					xrpc.WriteError(opts.Logger, w, err, "")
					return
				}
				ctx = storeUser(ctx, &xrpc.Auth{Handle: username})
			default:
				err := &xrpc.ErrorResponse{Message: "Auth required", Code: xrpc.AuthRequired}
				xrpc.WriteError(opts.Logger, w, err, xrpc.Forbidden)
				return
			}
			r = r.WithContext(ctx)
			h.ServeHTTP(w, r)
		})
	}
}

func RefreshTokenOnly(opts *Opts) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			raw, tt := getRawToken(r)
			switch tt {
			case bearer:
				tok, did, err := validateBearerToken(raw, ScopeRefresh, opts.secret)
				if err != nil {
					xrpc.WriteError(opts.Logger, w, err, "")
					return
				}
				ctx = storeUser(ctx, &xrpc.Auth{DID: did})
				ctx = storeToken(ctx, tok)
			default:
				err := &xrpc.ErrorResponse{Code: xrpc.AuthRequired}
				xrpc.WriteError(opts.Logger, w, err, xrpc.Forbidden)
				return
			}
			r = r.WithContext(ctx)
			h.ServeHTTP(w, r)
		})
	}
}

func ServiceJwt(opts *Opts) Middelware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			raw, tt := getRawToken(r)
			switch tt {
			case empty:
				err := xrpc.NewAuthRequired("Empty auth")
				xrpc.WriteError(opts.Logger, w, err, xrpc.AuthRequired)
				return
			case invalid:
				err := xrpc.NewInvalidRequest("Invalid auth token")
				xrpc.WriteError(opts.Logger, w, err, xrpc.InvalidRequest)
				return
			case basic:
				username, err := validateBasicAuth(opts, raw)
				if err != nil {
					xrpc.WriteError(opts.Logger, w, err, "")
					return
				}
				ctx = storeUser(ctx, &xrpc.Auth{Handle: username})
			case bearer:
				tok, did, err := validateBearerToken(raw, "", func(t *jwt.Token) (interface{}, error) {
					iss, err := t.Claims.GetIssuer()
					if err != nil {
						return nil, err
					}
					key, err := GetResolverSigningKey(ctx, opts.Resolver, iss)
					if err != nil {
						return nil, err
					}
					return key, nil
				})
				if err != nil {
					xrpc.WriteError(opts.Logger, w, err, "")
					return
				}
				ctx = storeUser(ctx, &xrpc.Auth{DID: did})
				ctx = storeToken(ctx, tok)
			}
			r = r.WithContext(ctx)
			h.ServeHTTP(w, r)
		})
	}
}

func GetResolverSigningKey(
	ctx context.Context,
	resolver indigodid.Resolver,
	iss string,
) (crypto.PublicKey, error) {
	did, serviceId := split2(iss, '#')
	var keyId string
	if serviceId == "atproto_labeler" {
		keyId = "atproto_label"
	} else {
		keyId = "atproto"
	}
	doc, err := resolver.GetDocument(ctx, did)
	if err != nil {
		return nil, err
	}
	return publicKeyFromDidDoc(doc, keyId)
}

func validateBearerToken(raw string, expectedScope Scope, keyfn jwt.Keyfunc) (*jwt.Token, string, error) {
	claims := make(jwt.MapClaims)
	tok, err := jwt.ParseWithClaims(raw, &claims, keyfn)
	if err != nil {
		return nil, "", xrpc.NewInvalidRequest("Invalid token").Wrap(err)
	}
	if !tok.Valid {
		return nil, "", xrpc.NewInvalidRequest("Invalid token")
	}

	if len(expectedScope) > 0 {
		scopeAny, ok := claims["scope"]
		if !ok {
			return nil, "", xrpc.NewInvalidRequest("Invalid JWT claims")
		}
		scope, ok := scopeAny.(string)
		if !ok {
			return nil, "", xrpc.NewInvalidRequest("Invalid JWT claims")
		}
		if Scope(scope) != expectedScope {
			return nil, "", xrpc.NewInvalidRequest("Expected refresh token")
		}
	}

	sub, err := claims.GetSubject()
	if err != nil {
		return nil, "", xrpc.NewInvalidRequest("JWT claims has no sub.").Wrap(err)
	}
	return tok, sub, nil
}

func validateBasicAuth(opts *Opts, raw string) (string, error) {
	opts.Logger.Info("Using basic auth")
	username, password, err := parseBasicAuth(raw)
	if err != nil {
		return "", xrpc.NewInvalidRequest("Invalid admin auth").Wrap(err)
	}
	if username != "admin" || password != opts.AdminPassword {
		if username != "admin" {
			opts.Logger.Warn("incorrect username", "username", username)
		}
		if password != opts.AdminPassword {
			opts.Logger.Warn("incorrect admin password")
		}
		return "", &xrpc.ErrorResponse{Message: "Invalid auth", Code: xrpc.Forbidden}
	}
	return username, nil
}

func publicKeyFromDidDoc(doc *did.Document, keyId string) (crypto.PublicKey, error) {
	if keyId == "" {
		keyId = "atproto"
	}
	vermeth, err := getVerificationMethod(doc, keyId)
	if err != nil {
		return nil, err
	}
	switch vermeth.Type {
	case "Multikey":
		if vermeth.PublicKeyMultibase == nil {
			return nil, errors.New("verification method type does not match actual payload")
		}
		return crypto.ParsePublicMultibase(*vermeth.PublicKeyMultibase)
	default:
		return nil, errors.Errorf("unknown verification method type %q", vermeth.Type)
	}
}

func getVerificationMethod(doc *did.Document, keyId string) (*did.VerificationMethod, error) {
	for i := range doc.VerificationMethod {
		_, serviceID := split2(doc.VerificationMethod[i].ID, '#')
		if serviceID == keyId {
			return &doc.VerificationMethod[i], nil
		}
	}
	return nil, errors.New("could not find verification method")
}

func split2(s string, sep byte) (string, string) {
	parts := strings.SplitN(s, string(sep), 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// ExtractToken is a middleware function that will store a jwt token in the
// request context if one is present. It will not fail if the jwt token is
// invalid or not found.
func ExtractToken(jwtSecret []byte) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := strings.ToLower(r.Header.Get("Authorization"))
			token, err := getToken(jwtSecret, auth)
			if err != nil {
				h.ServeHTTP(w, r)
				return
			}
			ctx := storeToken(r.Context(), token)
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TokenFromContext(ctx context.Context) *jwt.Token {
	val := ctx.Value(tokenKey)
	if val == nil {
		return nil
	}
	tok, ok := val.(*jwt.Token)
	if !ok {
		return nil
	}
	return tok
}

func UserFromContext(ctx context.Context) *xrpc.Auth {
	val := ctx.Value(userKey)
	if val == nil {
		return nil
	}
	u, ok := val.(*xrpc.Auth)
	if !ok {
		return nil
	}
	return u
}

func StashUser(ctx context.Context, auth *xrpc.Auth) context.Context {
	return context.WithValue(ctx, userKey, auth)
}

func storeToken(ctx context.Context, token *jwt.Token) context.Context {
	return context.WithValue(ctx, tokenKey, token)
}

func storeUser(ctx context.Context, user *xrpc.Auth) context.Context {
	return context.WithValue(ctx, userKey, user)
}

func getToken(jwtSecret []byte, authorizaiton string) (*jwt.Token, error) {
	token, found := strings.CutPrefix(strings.ToLower(authorizaiton), "bearer ")
	if !found {
		return nil, &xrpc.ErrorResponse{Code: xrpc.AuthRequired}
	}
	claims := make(jwt.MapClaims)
	tok, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil {
		return nil, xrpc.NewInvalidRequest("Invalid token").Wrap(err)
	}
	scope, ok := claims["scope"]
	if !ok {
		return nil, xrpc.NewInvalidRequest("Token scope required")
	}
	if s, ok := scope.(Scope); !ok || s != ScopeAccess {
		return nil, xrpc.NewInvalidRequest("Token scope should be an access token scope")
	}
	return tok, nil
}

func getRawToken(r *http.Request) (string, tokenType) {
	h := r.Header.Get("Authorization")
	if len(h) == 0 {
		return "", empty
	}
	h = string(unicode.ToLower(rune(h[0]))) + h[1:]
	v, found := strings.CutPrefix(h, "bearer ")
	if found {
		return v, bearer
	}
	v, found = strings.CutPrefix(h, "basic ")
	if found {
		return v, basic
	}
	return "", invalid
}

type tokenType uint

const (
	invalid tokenType = iota
	empty
	bearer
	basic
)

func parseBasicAuth(rawToken string) (username, password string, err error) {
	// Decode the Base64 portion of the header
	payload, err := base64.StdEncoding.DecodeString(rawToken)
	if err != nil {
		return "", "", xrpc.NewInvalidRequest("Invalid basic auth, could not decode base64")
	}
	// Split the decoded string into username and password
	parts := strings.SplitN(string(payload), ":", 2)
	if len(parts) != 2 {
		return "", "", xrpc.NewInvalidRequest("Invalid basic auth payload, missing ':'")
	}
	return parts[0], parts[1], nil
}
