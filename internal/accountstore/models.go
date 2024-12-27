package accountstore

import "database/sql"

// Account represents a row in the "account" table.
type Account struct {
	DID              string
	Email            string
	PasswordScrypt   string
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

// AppPassword represents a row in the "app_password" table.
type AppPassword struct {
	DID            string
	Name           string
	PasswordScrypt string
	CreatedAt      string
	Privileged     bool
}

// InviteCode represents a row in the "invite_code" table.
type InviteCode struct {
	Code          string
	AvailableUses int
	Disabled      bool
	ForAccount    string
	CreatedBy     string
	CreatedAt     string
}

// InviteCodeUse represents a row in the "invite_code_use" table.
type InviteCodeUse struct {
	Code   string
	UsedBy string
	UsedAt string
}

// RefreshToken represents a row in the "refresh_token" table.
type RefreshTokenModel struct {
	ID              string         `json:"jti"`
	DID             string         `json:"sub"`
	ExpiresAt       int64          `json:"exp"`
	NextID          sql.NullString `json:"-"`
	AppPasswordName sql.NullString `json:"-"`
}

// RepoRoot represents a row in the "repo_root" table.
type RepoRoot struct {
	DID       string
	CID       string
	Rev       string
	IndexedAt string
}

// EmailToken represents a row in the "email_token" table.
type EmailToken struct {
	Purpose     string
	DID         string
	Token       string
	RequestedAt string
}

// AuthorizationRequest represents a row in the "authorization_request" table.
type AuthorizationRequest struct {
	ID         string
	DID        sql.NullString
	DeviceID   sql.NullString
	ClientID   string
	ClientAuth string
	Parameters string
	ExpiresAt  string
	Code       sql.NullString
}

// Device represents a row in the "device" table.
type Device struct {
	ID         string
	SessionID  string
	UserAgent  sql.NullString
	IPAddress  string
	LastSeenAt string
}

// DeviceAccount represents a row in the "device_account" table.
type DeviceAccount struct {
	DID               string
	DeviceID          string
	AuthenticatedAt   string
	Remember          bool
	AuthorizedClients string
}

// Token represents a row in the "token" table.
type Token struct {
	ID                  int
	DID                 string
	TokenID             string
	CreatedAt           string
	UpdatedAt           string
	ExpiresAt           string
	ClientID            string
	ClientAuth          string
	DeviceID            sql.NullString
	Parameters          string
	Details             sql.NullString
	Code                sql.NullString
	CurrentRefreshToken sql.NullString
}
