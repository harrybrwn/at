CREATE TABLE IF NOT EXISTS "app_password" (
  "did" varchar not null,
  "name" varchar not null,
  "passwordScrypt" varchar not null,
  "createdAt" varchar not null,
  "privileged" integer default 0 not null,
  constraint "app_password_pkey" primary key ("did", "name")
);

CREATE TABLE IF NOT EXISTS "invite_code" (
  "code" varchar primary key,
  "availableUses" integer not null,
  "disabled" int2 default 0,
  "forAccount" varchar not null,
  "createdBy" varchar not null,
  "createdAt" varchar not null
);

CREATE INDEX IF NOT EXISTS "invite_code_for_account_idx" on "invite_code" ("forAccount");

CREATE TABLE IF NOT EXISTS "invite_code_use" (
  "code" varchar not null,
  "usedBy" varchar not null,
  "usedAt" varchar not null,
  constraint "invite_code_use_pkey" primary key ("code", "usedBy")
);

CREATE TABLE IF NOT EXISTS "refresh_token" (
  "id" varchar primary key,
  "did" varchar not null,
  "expiresAt" varchar not null,
  "nextId" varchar,
  "appPasswordName" varchar
);

CREATE INDEX IF NOT EXISTS "refresh_token_did_idx" on "refresh_token" ("did");

CREATE TABLE IF NOT EXISTS "repo_root" (
  "did" varchar primary key,
  "cid" varchar not null,
  "rev" varchar not null,
  "indexedAt" varchar not null
);

CREATE TABLE IF NOT EXISTS "actor" (
  "did" varchar primary key,
  "handle" varchar,
  "createdAt" varchar not null,
  "takedownRef" varchar,
  "deactivatedAt" varchar,
  "deleteAfter" varchar
);

CREATE UNIQUE INDEX IF NOT EXISTS "actor_handle_lower_idx" on "actor" (lower("handle"));

CREATE INDEX IF NOT EXISTS "actor_cursor_idx" on "actor" ("createdAt", "did");

CREATE TABLE IF NOT EXISTS "account" (
  "did" varchar primary key,
  "email" varchar not null,
  "passwordScrypt" varchar not null,
  "emailConfirmedAt" varchar,
  "invitesDisabled" int2 default 0 not null
);

CREATE UNIQUE INDEX IF NOT EXISTS "account_email_lower_idx" on "account" (lower("email"));

CREATE TABLE IF NOT EXISTS "email_token" (
  "purpose" varchar not null,
  "did" varchar not null,
  "token" varchar not null,
  "requestedAt" varchar not null,
  constraint "email_token_pkey" primary key ("purpose", "did"),
  constraint "email_token_purpose_token_unique" unique ("purpose", "token")
);

CREATE TABLE IF NOT EXISTS "authorization_request" (
  "id" varchar primary key,
  "did" varchar,
  "deviceId" varchar,
  "clientId" varchar not null,
  "clientAuth" varchar not null,
  "parameters" varchar not null,
  "expiresAt" varchar not null,
  "code" varchar
);

CREATE UNIQUE INDEX IF NOT EXISTS "authorization_request_code_idx" on "authorization_request" (code DESC)
WHERE
  (code IS NOT NULL);

CREATE INDEX IF NOT EXISTS "authorization_request_expires_at_idx" on "authorization_request" ("expiresAt");

CREATE TABLE IF NOT EXISTS "device" (
  "id" varchar primary key,
  "sessionId" varchar not null,
  "userAgent" varchar,
  "ipAddress" varchar not null,
  "lastSeenAt" varchar not null,
  constraint "device_session_id_idx" unique ("sessionId")
);

CREATE TABLE IF NOT EXISTS "device_account" (
  "did" varchar not null,
  "deviceId" varchar not null,
  "authenticatedAt" varchar not null,
  "remember" boolean not null,
  "authorizedClients" varchar not null,
  constraint "device_account_pk" primary key ("deviceId", "did"),
  constraint "device_account_device_id_fk" foreign key ("deviceId") references "device" ("id") on delete cascade on update cascade
);

CREATE TABLE IF NOT EXISTS "token" (
  "id" integer primary key autoincrement,
  "did" varchar not null,
  "tokenId" varchar not null,
  "createdAt" varchar not null,
  "updatedAt" varchar not null,
  "expiresAt" varchar not null,
  "clientId" varchar not null,
  "clientAuth" varchar not null,
  "deviceId" varchar,
  "parameters" varchar not null,
  "details" varchar,
  "code" varchar,
  "currentRefreshToken" varchar,
  constraint "token_current_refresh_token_unique_idx" unique ("currentRefreshToken"),
  constraint "token_id_unique_idx" unique ("tokenId")
);

-- CREATE TABLE sqlite_sequence (name, seq);

CREATE INDEX IF NOT EXISTS "token_did_idx" on "token" ("did");

CREATE UNIQUE INDEX IF NOT EXISTS "token_code_idx" on "token" (code DESC)
WHERE
  (code IS NOT NULL);

CREATE TABLE IF NOT EXISTS "used_refresh_token" (
  "refreshToken" varchar primary key,
  "tokenId" integer not null,
  constraint "used_refresh_token_fk" foreign key ("tokenId") references "token" ("id") on delete cascade on update cascade
);

CREATE INDEX IF NOT EXISTS "used_refresh_token_id_idx" on "used_refresh_token" ("tokenId");
