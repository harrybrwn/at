package accountstore

import (
	"context"
	"time"

	"github.com/harrybrwn/db"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/internal/auth"
	"github.com/harrybrwn/at/internal/parallel"
)

// CreateAccountOptions represents the options to create an account
type CreateAccountOpts struct {
	DID         string
	Handle      string
	Email       *string
	Password    *string
	RepoCid     string
	RepoRev     string
	InviteCode  *string
	Deactivated *bool
}

type FreshAccount struct {
	// account.ActorAccount
	AccessJwt, RefreshJwt string
}

// CreateAccount method to create a new account and actor.
func (as *AccountStore) CreateAccount(ctx context.Context, opts CreateAccountOpts) (accessJwt string, refreshJwt string, err error) {
	did := opts.DID
	handle := opts.Handle
	email := opts.Email
	repoCid := opts.RepoCid
	repoRev := opts.RepoRev
	deactivated := opts.Deactivated

	// Hash the password using bcrypt
	var passwordHash []byte
	if opts.Password != nil {
		hash, err := hashPassword(*opts.Password)
		if err != nil {
			return "", "", errors.Wrap(err, "failed to hash password")
		}
		passwordHash = hash
	}
	now := time.Now().UTC()

	// Generate JWT tokens (mocked function)
	accessJwt, refreshJwt, err = auth.CreateTokens(&auth.CreateTokenOpts{
		DID:        did,
		JWTKey:     as.jwtKey,
		ServiceDID: as.serviceDID,
		Now:        &now,
	})
	if err != nil {
		return "", "", errors.Wrap(err, "failed to create tokens")
	}
	refreshPayload, err := decodeRefreshToken(refreshJwt, as.jwtKey)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to decode refresh token")
	}

	tx, err := as.db.Begin()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to start transaction")
	}
	dbtx := db.NewTx(tx)
	jobs := parallel.BasicJobs{
		// Register actor
		func(ctx context.Context) error {
			return registerActor(ctx, dbtx, did, handle, deactivated)
		},
		// Store refresh token
		func(ctx context.Context) error {
			return storeRefreshToken(ctx, dbtx, refreshPayload, nil)
		},
		// Update root
		func(ctx context.Context) error {
			return updateRoot(ctx, dbtx, did, repoCid, repoRev, now)
		},
	}
	if opts.InviteCode != nil {
		// Ensure invite is available if inviteCode is provided
		jobs.Add(func(ctx context.Context) error {
			return ensureInviteIsAvailable(ctx, dbtx, *opts.InviteCode)
		})
		// Record invite use if applicable
		jobs.Add(func(ctx context.Context) error {
			return recordInviteUse(ctx, dbtx, did, *opts.InviteCode, now)
		})
	}
	// Register account if email and password are provided
	if email != nil && passwordHash != nil {
		jobs.Add(func(ctx context.Context) error {
			return registerAccount(ctx, dbtx, did, *email, passwordHash)
		})
	}
	err = parallel.Do(ctx, jobs...)
	if err != nil {
		_ = tx.Rollback()
		return "", "", err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return "", "", errors.Wrap(err, "failed to commit transaction")
	}
	return accessJwt, refreshJwt, nil
}

// registerActor creates a new actor entry in the database
func registerActor(ctx context.Context, tx db.DB, did, handle string, deactivated *bool) error {
	var (
		createdAt     = time.Now().UTC().Format(time.RFC3339)
		deactivatedAt *string
		deleteAfter   *string
	)
	if deactivated != nil && *deactivated {
		deactivatedAt = &createdAt
		deleteAfter = new(string)
		*deleteAfter = time.Now().Add(3 * 24 * time.Hour).UTC().Format(time.RFC3339)
	}

	query := `INSERT INTO actor (
		did,
		handle,
		createdAt,
		deactivatedAt,
		deleteAfter
	) VALUES (?, ?, ?, ?, ?)
	ON CONFLICT DO NOTHING`
	_, err := tx.ExecContext(ctx, query, did, handle, createdAt, deactivatedAt, deleteAfter)
	if err != nil {
		return errors.Wrap(err, "failed to register actor")
	}
	return nil
}

// registerAccount creates a new account entry in the database
func registerAccount(ctx context.Context, tx db.DB, did, email string, passwordHash []byte) error {
	query := `INSERT INTO account (did, email, passwordScrypt) 
			  VALUES (?, ?, ?)
			  ON CONFLICT DO NOTHING`
	_, err := tx.ExecContext(ctx, query, did, email, passwordHash)
	if err != nil {
		return errors.Wrap(err, "failed to register account")
	}
	return nil
}

// ensureInviteIsAvailable checks if the invite code is available for use
func ensureInviteIsAvailable(ctx context.Context, tx db.DB, inviteCode string) error {
	query := `SELECT availableUses, disabled FROM invite_code 
			  LEFT JOIN actor ON actor.did = invite_code.forAccount
			  WHERE code = ? AND takedownRef IS NULL`
	var availableUses, disabled int
	rows, err := tx.QueryContext(ctx, query, inviteCode)
	if err != nil {
		return errors.Wrap(err, "failed to ensure invite is available")
	}
	err = db.ScanOne(rows, &availableUses, &disabled)
	if err != nil {
		return errors.Wrap(err, "failed to ensure invite is available")
	}
	if disabled == 1 || availableUses <= 0 {
		return errors.New("invite code not available")
	}

	// Check how many times the invite has been used
	var usesCount int
	query = `SELECT COUNT(*) FROM invite_code_use WHERE code = ?`
	rows, err = tx.QueryContext(ctx, query, inviteCode)
	if err != nil {
		return errors.Wrap(err, "failed to count invite uses")
	}
	if err = db.ScanOne(rows, &usesCount); err != nil {
		return errors.Wrap(err, "failed to count invite uses")
	}
	if availableUses <= usesCount {
		return errors.New("invite code not available")
	}
	return nil
}

// updateRoot updates the root of the repo
func updateRoot(ctx context.Context, tx db.DB, did, cid, rev string, now time.Time) error {
	query := `INSERT OR REPLACE INTO repo_root (did, cid, rev, indexedAt) 
			  VALUES (?, ?, ?, ?)`
	_, err := tx.ExecContext(
		ctx,
		query,
		did,
		cid,
		rev,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return errors.Wrap(err, "failed to update repo root")
	}
	return nil
}
