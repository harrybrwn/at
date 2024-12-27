package account

import "database/sql"

type Status uint8

const (
	StatusNone Status = iota
	StatusActive
	StatusTakendown
	StatusSuspended
	StatusDeleted
	StatusDeactivated
)

func (as Status) String() string {
	switch as {
	case StatusActive:
		return "active"
	case StatusTakendown:
		return "takendown"
	case StatusSuspended:
		return "suspended"
	case StatusDeleted:
		return "deleted"
	case StatusDeactivated:
		return "deactivated"
	}
	return ""
}

type ActorAccount struct {
	Actor
	Email            string
	EmailConfirmedAt sql.NullString
	InvitesDisabled  bool
}

// Actor represents a row in the "actor" table.
type Actor struct {
	DID           string
	Handle        sql.NullString
	CreatedAt     string
	TakedownRef   sql.NullString
	DeactivatedAt sql.NullString
	DeleteAfter   sql.NullString
}
