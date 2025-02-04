package accountstore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/harrybrwn/db"
	"github.com/pkg/errors"
)

func getRefreshTokenByDID(ctx context.Context, d db.DB, did string) (*RefreshTokenModel, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, did, expiresAt, nextId, appPasswordName FROM refresh_token WHERE did = ?`, did)
	if err != nil {
		return nil, err
	}
	var rt RefreshTokenModel
	err = db.ScanOne(
		rows,
		&rt.ID,
		&rt.DID,
		&rt.ExpiresAt,
		&rt.NextID,
		&rt.AppPasswordName,
	)
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

func storeRefreshToken(ctx context.Context, d db.DB, token *RefreshTokenModel, _ any) error {
	_, err := d.ExecContext(
		ctx,
		`INSERT INTO refresh_token (
            id,
            did,
            expiresAt,
            nextId,
            appPasswordName
        ) VALUES (?, ?, ?, ?, ?)`,
		token.ID,
		token.DID,
		token.ExpiresAt,
		token.NextID,
		token.AppPasswordName,
	)
	return err
}

func decodeRefreshToken(refreshToken string, jwtkey []byte) (*RefreshTokenModel, error) {
	parts := strings.SplitN(refreshToken, ".", 3)
	if len(parts) < 2 {
		return nil, errors.New("invalid refresh token")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode jwt base64")
	}
	var rt RefreshTokenModel
	err = json.Unmarshal(body, &rt)
	if err != nil {
		return nil, err
	}
	return &rt, nil
}
