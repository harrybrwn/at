package accountstore

import (
	"context"
	"database/sql"
	_ "embed"
	"strings"
	"time"

	"github.com/harrybrwn/db"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/internal/auth"
)

//go:embed init.sql
var migration []byte

type AccountStore struct {
	db         *sql.DB
	jwtKey     []byte
	serviceDID string
}

func New(db *sql.DB, jwtKey []byte, serviceDID string) *AccountStore {
	return &AccountStore{db: db, jwtKey: jwtKey, serviceDID: serviceDID}
}

func (as *AccountStore) Close() error {
	return as.db.Close()
}

func (as *AccountStore) Migrate(ctx context.Context) error {
	_, err := as.db.ExecContext(ctx, string(migration))
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = as.db.ExecContext(ctx, `PRAGMA journal_mode = WAL`)
	return errors.WithStack(err)
}

// updateEmail updates the email for a given `did` in the "account" table.
func (as *AccountStore) UpdateEmail(ctx context.Context, did string, email string) error {
	// Prepare the email (ensure it's lowercase)
	email = strings.ToLower(email)
	// Execute the update query
	query := `UPDATE account SET email = ?, emailConfirmedAt = NULL WHERE did = ?`
	_, err := as.db.ExecContext(ctx, query, email, did)
	if err != nil {
		// TODO Handle unique constraint violation (if any)
		//if isErrUniqueViolation(err) {
		//	return &UserAlreadyExistsError{Message: "Email already exists"}
		//}
		return errors.Wrap(err, "failed to update email")
	}
	return nil
}

func (as *AccountStore) UpdateRoot(ctx context.Context, did, cid, rev string) error {
	return updateRoot(ctx, db.Simple(as.db), did, cid, rev, time.Now().UTC())
}

func (as *AccountStore) UpdateHandle(ctx context.Context, did string, handle string) error {
	// Check if handle already exists for another actor
	var count int
	query := `SELECT COUNT(*) FROM actor WHERE handle = ?`
	err := as.db.QueryRowContext(ctx, query, handle).Scan(&count)
	if err != nil {
		return errors.Wrap(err, "failed to check if handle exists")
	}
	// If handle exists, return an error
	if count > 0 {
		return errors.New("handle already exists")
	}

	// Perform the update
	updateQuery := `UPDATE actor SET handle = ? WHERE did = ?`
	res, err := as.db.ExecContext(ctx, updateQuery, handle, did)
	if err != nil {
		return errors.Wrap(err, "failed to update handle")
	}

	// Check if any row was updated
	updatedRows, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to get affected rows")
	}
	if updatedRows < 1 {
		return errors.New("no rows updated")
	}
	return nil
}

// TODO In atproto/packages/pds/src/account-manager/index.ts, createSession
// accepts an `appPassword` type and this should be supported in the future.
func (as *AccountStore) CreateSession(
	ctx context.Context,
	did string,
	appPassword *AppPassDescript,
) (accessJwt string, refreshJwt string, err error) {
	now := time.Now().UTC()
	accessJwt, refreshJwt, err = auth.CreateTokens(&auth.CreateTokenOpts{
		DID:        did,
		JWTKey:     as.jwtKey,
		ServiceDID: as.serviceDID,
		Now:        &now,
	})
	if err != nil {
		return "", "", err
	}
	refreshTokenData, err := decodeRefreshToken(refreshJwt, as.jwtKey)
	if err != nil {
		return "", "", err
	}
	err = storeRefreshToken(ctx, db.Simple(as.db), refreshTokenData, appPassword)
	return accessJwt, refreshJwt, err
}

func (as *AccountStore) EnsureInviteIsAvailable(ctx context.Context, code string) error {
	return ensureInviteIsAvailable(ctx, db.New(as.db), code)
}

// StatusAttr defines the structure for takedown status
type StatusAttr struct {
	Applied bool
	Ref     *string // Ref is a pointer to allow null values
}

func updateAccountTakedownStatus(ctx context.Context, tx db.DB, did string, takedown *StatusAttr) error {
	// Determine takedownRef based on the applied status
	var takedownRef any
	if takedown.Applied {
		if takedown.Ref != nil {
			takedownRef = *takedown.Ref
		} else {
			// If ref is nil, use the current timestamp
			takedownRef = time.Now().UTC().Format(time.RFC3339)
		}
	} else {
		takedownRef = nil
	}
	// Prepare the SQL query to update the actor's takedownRef
	query := `UPDATE actor SET takedownRef = ? WHERE did = ?`
	_, err := tx.ExecContext(ctx, query, takedownRef, did)
	if err != nil {
		return errors.Wrap(err, "failed to update takedown status")
	}
	return nil
}

type accountAdminStatus struct {
	Takedown    StatusAttr
	Deactivated StatusAttr
}

// getAccountAdminStatus retrieves the takedown and deactivated statuses for a given DID.
func getAccountAdminStatus(ctx context.Context, d db.DB, did string) (*accountAdminStatus, error) {
	// Query to select takedownRef and deactivatedAt from the actor table
	query := `SELECT takedownRef, deactivatedAt FROM actor WHERE did = ?`

	var takedownRef sql.NullString
	var deactivatedAt sql.NullString

	// Execute the query with context
	rows, err := d.QueryContext(ctx, query, did)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve account admin status")
	}
	err = db.ScanOne(rows, &takedownRef, &deactivatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			// If no rows are found, return nil without an error
			return nil, nil
		}
		// For other errors, return the error
		return nil, errors.Wrap(err, "failed to retrieve account admin status")
	}

	takedown := StatusAttr{
		Applied: takedownRef.Valid,
	}
	if takedownRef.Valid {
		takedown.Ref = &takedownRef.String
	}

	deactivated := StatusAttr{
		Applied: deactivatedAt.Valid,
	}

	// Return the result
	return &accountAdminStatus{
		Takedown:    takedown,
		Deactivated: deactivated,
	}, nil
}

func executeWithRetry(ctx context.Context, fn func(ctx context.Context) error) error {
	// TODO implement retries
	return fn(ctx)
}
