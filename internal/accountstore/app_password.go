package accountstore

import (
	"bytes"
	"context"
	"time"

	database "github.com/harrybrwn/db"
	"github.com/pkg/errors"
)

func (as *AccountStore) CreateAppPassword(ctx context.Context, did, name string, privilaged bool) (*NuevoAppPassword, error) {
	return createAppPassword(ctx, database.Simple(as.db), did, name, privilaged)
}

func (as *AccountStore) DeleteAppPassword(ctx context.Context, did, name string) error {
	return deleteAppPassword(ctx, database.Simple(as.db), did, name)
}

type AppPassDescript struct {
	Name       string
	Privileged bool
}

func (as *AccountStore) VerifyAppPassword(ctx context.Context, did, password string) (*AppPassDescript, error) {
	scryptHash, err := hashAppPassword(did, []byte(password))
	if err != nil {
		return nil, err
	}
	rows, err := as.db.QueryContext(ctx, "SELECT name, privileged FROM app_password WHERE passwordScrypt = ?", string(scryptHash))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var ap AppPassDescript
	err = database.ScanOne(rows, &ap.Name, &ap.Privileged)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &ap, nil
}

// NuevoAppPassword is a newly created app password.
type NuevoAppPassword struct {
	DID        string
	Name       string
	Password   []byte
	CreatedAt  time.Time
	Privileged bool
}

func createAppPassword(ctx context.Context, db database.DB, did, name string, privilaged bool) (*NuevoAppPassword, error) {
	gen, err := genRandomBytes(16)
	if err != nil {
		return nil, err
	}
	chunks := [][]byte{gen[:4], gen[4:8], gen[8:12], gen[12:16]}
	password := bytes.Join(chunks, []byte{'-'})
	pwScrypt, err := hashAppPassword(did, password)
	if err != nil {
		return nil, err
	}
	createdAt := time.Now().UTC()
	_, err = db.ExecContext(
		ctx,
		`INSERT INTO app_password (
			did, name, passwordScrypt,
			createdAt, privileged
		) VALUES (?, ?, ?, ?, ?)`,
		did, name, pwScrypt,
		createdAt.Format(time.RFC3339),
		privilaged,
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &NuevoAppPassword{
		DID:        did,
		Name:       name,
		Password:   password,
		CreatedAt:  createdAt,
		Privileged: privilaged,
	}, nil
}

func deleteAppPassword(ctx context.Context, db database.DB, did, name string) error {
	return executeWithRetry(ctx, func(ctx context.Context) error {
		_, err := db.ExecContext(ctx, "DELETE FROM app_password WHERE did = ? AND name = ?", did, name)
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}
