package accountstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/harrybrwn/db"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/xrpc"
)

func (as *AccountStore) CreateInviteCode(ctx context.Context, codes []string, forAccount string, useCount int) error {
	for _, code := range codes {
		_, err := as.db.ExecContext(ctx, `
			INSERT INTO invite_code (
				code,
				availableUses,
				disabled,
				forAccount,
				createdBy,
				createdAt
			) VALUES (?, ?, ?, ?, ?, ?)`,
			code,
			useCount,
			0,
			forAccount,
			"admin",
			time.Now().UTC().Format(time.RFC3339),
		)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (as *AccountStore) RecordInviteUse(ctx context.Context, code, usedBy string) error {
	return recordInviteUse(
		ctx,
		db.Simple(as.db),
		usedBy,
		code,
		time.Now().UTC(),
	)
}

// EnsureInviteIsAvailable checks if an invite code is valid and available
func (as *AccountStore) EnsureInviteAvailable(ctx context.Context, inviteCode string) error {
	return EnsureInviteIsAvailable(ctx, as.db, inviteCode)
}

// EnsureInviteIsAvailable checks if an invite code is valid and available
func EnsureInviteIsAvailable(ctx context.Context, db *sql.DB, inviteCode string) error {
	inviteQuery := `
		SELECT ic.code, ic.forAccount, ic.disabled, ic.availableUses
		FROM invite_code ic
		LEFT JOIN actor a ON a.did = ic.forAccount
		WHERE a.takedownRef IS NULL
			  AND ic.code = ?`
	var invite InviteCode
	err := db.QueryRowContext(ctx, inviteQuery, inviteCode).Scan(
		&invite.Code,
		&invite.ForAccount,
		&invite.Disabled,
		&invite.AvailableUses,
	)
	if err == sql.ErrNoRows || invite.Disabled {
		return &xrpc.ErrorResponse{
			Message: "Provided invite code not available",
			Code:    "InvalidInviteCode",
			Inner:   errors.WithStack(err),
		}
	}
	if err != nil {
		return errors.Wrap(err, "error querying invite code")
	}

	// Query to count uses of the invite code
	usesQuery := `
        SELECT COUNT(*) as count
        FROM invite_code_use
        WHERE code = ?`
	var useCount int
	err = db.QueryRowContext(ctx, usesQuery, inviteCode).Scan(&useCount)
	if err != nil {
		return errors.Wrap(err, "error counting invite code uses")
	}
	if invite.AvailableUses <= useCount {
		return &xrpc.ErrorResponse{
			Message: "Provided invite code not available",
			Code:    "InvalidInviteCode",
		}
	}
	return nil
}

// recordInviteUse records the use of an invite code
func recordInviteUse(ctx context.Context, tx db.DB, did, inviteCode string, now time.Time) error {
	if inviteCode == "" {
		return nil
	}
	query := `INSERT INTO invite_code_use (
		code,
		usedBy,
		usedAt
	) VALUES (?, ?, ?)`
	_, err := tx.ExecContext(ctx, query, inviteCode, did, now.Format(time.RFC3339))
	if err != nil {
		return errors.Wrap(err, "failed to record invite use")
	}
	return nil
}
