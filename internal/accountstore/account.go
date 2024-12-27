package accountstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/harrybrwn/db"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/internal/account"
)

type GetAccountOpts struct {
	IncludeTakenDown   bool
	IncludeDeactivated bool
}

func (o *GetAccountOpts) WithTakenDown() *GetAccountOpts {
	o.IncludeTakenDown = true
	return o
}

func (o *GetAccountOpts) WithDeactivated() *GetAccountOpts {
	o.IncludeDeactivated = true
	return o
}

// GetAccount retrieves an account by DID.
func (as *AccountStore) GetAccount(ctx context.Context, identifier string, opts *GetAccountOpts) (*account.ActorAccount, error) {
	var (
		query strings.Builder
		where string
	)
	if strings.HasPrefix(identifier, "did:") {
		where = "actor.did = ?"
	} else {
		where = "actor.handle = ?"
	}
	if err := buildSelectAccount(opts, where, &query); err != nil {
		return nil, err
	}
	row := as.db.QueryRowContext(ctx, query.String(), identifier)
	var acct account.ActorAccount
	err := scanAccount(&acct, row)
	if err != nil {
		return nil, err
	}
	return &acct, nil
}

// GetAccounts retrieves all accounts and their corresponding actors.
func (as *AccountStore) GetAccounts(ctx context.Context, opts *GetAccountOpts) ([]*account.ActorAccount, error) {
	var query strings.Builder
	if err := buildSelectAccount(opts, "", &query); err != nil {
		return nil, err
	}
	rows, err := as.db.QueryContext(ctx, query.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]*account.ActorAccount, 0)
	for rows.Next() {
		var account account.ActorAccount
		err := scanAccount(&account, rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, &account)
	}
	return accounts, nil
}

// GetAccountByEmail retrieves an account and its corresponding actor by email.
func (as *AccountStore) GetAccountByEmail(ctx context.Context, email string, opts *GetAccountOpts) (*account.ActorAccount, error) {
	var query strings.Builder
	if err := buildSelectAccount(opts, "lower(account.email) = lower(?)", &query); err != nil {
		return nil, err
	}
	row := as.db.QueryRowContext(ctx, query.String(), email)
	var a account.ActorAccount
	return &a, scanAccount(&a, row)
}

func scanAccount(a *account.ActorAccount, row db.Scanner) error {
	var invitesDisabled int
	err := row.Scan(
		&a.DID,
		&a.Handle,
		&a.CreatedAt,
		&a.TakedownRef,
		&a.DeactivatedAt,
		&a.DeleteAfter,
		&a.Email,
		&a.EmailConfirmedAt,
		&invitesDisabled,
	)
	if err != nil {
		return errors.WithStack(err)
	}
	a.InvitesDisabled = invitesDisabled != 0
	return nil
}

func buildSelectAccount(opts *GetAccountOpts, where string, b *strings.Builder) error {
	_, err := b.WriteString(`SELECT
	actor.did,
	actor.handle,
	actor.createdAt,
	actor.takedownRef,
	actor.deactivatedAt,
	actor.deleteAfter,
	account.email,
	account.emailConfirmedAt,
	account.invitesDisabled
FROM account
LEFT JOIN actor ON account.did = actor.did `)
	if err != nil {
		return errors.WithStack(err)
	}
	if opts == nil {
		opts = new(GetAccountOpts)
	}
	var hasWhere bool
	if len(where) > 0 {
		_, err = b.WriteString(`WHERE `)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = b.WriteString(where)
		if err != nil {
			return errors.WithStack(err)
		}
		hasWhere = true
	}

	if !opts.IncludeTakenDown {
		if !hasWhere {
			_, err = b.WriteString(` WHERE `)
			hasWhere = true
		} else {
			_, err = b.WriteString(` AND `)
		}
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = b.WriteString(`actor."takedownRef" is null`)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	if !opts.IncludeDeactivated {
		if !hasWhere {
			_, err = b.WriteString(` WHERE `)
			// hasWhere = true
		} else {
			_, err = b.WriteString(` AND `)
		}
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = b.WriteString(`actor."deactivatedAt" is null`)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// IsAccountActivated checks if an account is activated.
func (as *AccountStore) IsAccountActivated(ctx context.Context, did string) (bool, error) {
	query := `SELECT emailConfirmedAt FROM account WHERE did = ?`
	var emailConfirmedAt sql.NullString
	if err := as.db.QueryRowContext(ctx, query, did).Scan(&emailConfirmedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}
	return emailConfirmedAt.Valid, nil
}

// TakedownAccount sets the takedown reference for an actor.
func (as *AccountStore) TakedownAccount(ctx context.Context, did string, takedown *StatusAttr) error {
	tx, err := as.db.Begin()
	if err != nil {
		return errors.Wrap(err, "failed to start transaction")
	}
	err = updateAccountTakedownStatus(ctx, db.NewTx(tx), did, takedown)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM refresh_token WHERE did = ?`, did)
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM token WHERE did = ?`, did)
	if err != nil {
		return errors.WithStack(err)
	}
	return err
}

// DeleteAccount deletes all records associated with the given `did` from several tables.
func (as *AccountStore) DeleteAccount(ctx context.Context, did string) error {
	// Execute delete operations
	tables := []string{
		"repo_root",
		"email_token",
		"refresh_token",
		"account",
		"actor",
	}
	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE did = ?", table)
		_, err := as.db.ExecContext(ctx, query, did)
		if err != nil {
			return errors.Wrapf(err, "failed to delete from %s", table)
		}
	}
	return nil
}

// getAccountAdminStatus retrieves the takedown and deactivated statuses for a given DID.
func (as *AccountStore) GetAccountAdminStatus(ctx context.Context, did string) (*accountAdminStatus, error) {
	return getAccountAdminStatus(ctx, db.Simple(as.db), did)
}

// GetAccountStatus retrieves the status of an account.
func (as *AccountStore) GetAccountStatus(ctx context.Context, did string) (account.Status, error) {
	query := `SELECT deactivatedAt FROM actor WHERE did = ?`
	var deactivatedAt sql.NullString
	if err := as.db.QueryRowContext(ctx, query, did).Scan(&deactivatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return account.StatusActive, nil
		}
		return account.StatusNone, errors.WithStack(err)
	}
	if deactivatedAt.Valid {
		return account.StatusDeactivated, nil
	}
	return account.StatusActive, nil
}

// DeactivateAccount deactivates an account.
func (as *AccountStore) DeactivateAccount(ctx context.Context, did string, deleteAfter sql.NullString) error {
	query := `UPDATE actor SET deactivatedAt = CURRENT_TIMESTAMP, deleteAfter = ? WHERE did = ?`
	_, err := as.db.ExecContext(ctx, query, deleteAfter, did)
	return errors.WithStack(err)
}

// ActivateAccount reactivates an account.
func (as *AccountStore) ActivateAccount(ctx context.Context, did string) error {
	query := `UPDATE actor SET deactivatedAt = NULL, deleteAfter = NULL WHERE did = ?`
	_, err := as.db.ExecContext(ctx, query, did)
	return errors.WithStack(err)
}

func (as *AccountStore) VerifyAccountPassword(ctx context.Context, did, password string) error {
	rows, err := as.db.QueryContext(ctx, "SELECT passwordScrypt FROM account WHERE did = ?", did)
	if err != nil {
		return errors.WithStack(err)
	}
	var storedHash []byte
	err = db.ScanOne(rows, &storedHash)
	if err != nil {
		return errors.WithStack(err)
	}
	return verifyPassword(password, storedHash)
}

func (as *AccountStore) accountCount(ctx context.Context) (int, error) {
	rows, err := as.db.QueryContext(ctx, `SELECT COUNT(*) FROM account`)
	if err != nil {
		return 0, err
	}
	var n int
	err = db.ScanOne(rows, &n)
	return n, err
}

func (as *AccountStore) dids(ctx context.Context) ([]string, error) {
	rows, err := as.db.QueryContext(ctx, `SELECT did FROM account`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := make([]string, 0)
	for rows.Next() {
		var did string
		err = rows.Scan(&did)
		if err != nil {
			return nil, err
		}
		res = append(res, did)
	}
	return res, nil
}
