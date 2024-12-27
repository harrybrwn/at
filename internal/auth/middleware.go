package auth

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	"github.com/golang-jwt/jwt/v5"

	"github.com/harrybrwn/at/xrpc"
)

type ContextKey string

const (
	tokenKey ContextKey = "xrpc-token"
	userKey  ContextKey = "xrpc-user"
)

type Opts struct {
	Logger        *slog.Logger
	JWTSecret     []byte
	AdminPassword string
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
			case invalid:
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
				claims := make(jwt.MapClaims)
				tok, err := jwt.ParseWithClaims(raw, &claims, opts.secret)
				if err != nil {
					xrpc.WriteInvalidRequest(opts.Logger, w, err, "Invalid token")
					return
				}
				if !tok.Valid {
					xrpc.WriteInvalidRequest(opts.Logger, w, nil, "Invalid token")
					return
				}
				sub, err := claims.GetSubject()
				if err != nil {
					xrpc.WriteInvalidRequest(opts.Logger, w, err, "JWT claims has no sub.")
					return
				}
				ctx = storeUser(ctx, &xrpc.Auth{
					DID: sub,
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
			case bearer:
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
		return "", invalid
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
