CREATE TABLE IF NOT EXISTS "repo_root" (
  "did" varchar primary key,
  "cid" varchar not null,
  "rev" varchar not null,
  "indexedAt" varchar not null
);

CREATE TABLE IF NOT EXISTS "repo_block" (
  "cid" varchar primary key,
  "repoRev" varchar not null,
  "size" integer not null,
  "content" blob not null
);

CREATE INDEX IF NOT EXISTS "repo_block_repo_rev_idx" on "repo_block" ("repoRev", "cid");

CREATE TABLE IF NOT EXISTS "record" (
  "uri" varchar primary key,
  "cid" varchar not null,
  "collection" varchar not null,
  "rkey" varchar not null,
  "repoRev" varchar not null,
  "indexedAt" varchar not null,
  "takedownRef" varchar
);

CREATE INDEX IF NOT EXISTS "record_cid_idx" on "record" ("cid");

CREATE INDEX IF NOT EXISTS "record_collection_idx" on "record" ("collection");

CREATE INDEX IF NOT EXISTS "record_repo_rev_idx" on "record" ("repoRev");

CREATE TABLE IF NOT EXISTS "blob" (
  "cid" varchar primary key,
  "mimeType" varchar not null,
  "size" integer not null,
  "tempKey" varchar,
  "width" integer,
  "height" integer,
  "createdAt" varchar not null,
  "takedownRef" varchar
);

CREATE INDEX IF NOT EXISTS "blob_tempkey_idx" on "blob" ("tempKey");

CREATE TABLE IF NOT EXISTS "record_blob" (
  "blobCid" varchar not null,
  "recordUri" varchar not null,
  constraint "record_blob_pkey" primary key ("blobCid", "recordUri")
);

CREATE TABLE IF NOT EXISTS "backlink" (
  "uri" varchar not null,
  "path" varchar not null,
  "linkTo" varchar not null,
  constraint "backlinks_pkey" primary key ("uri", "path")
);

CREATE INDEX IF NOT EXISTS "backlink_link_to_idx" on "backlink" ("path", "linkTo");

CREATE TABLE IF NOT EXISTS "account_pref" (
  "id" integer primary key autoincrement,
  "name" varchar not null,
  "valueJson" text not null
);

-- CREATE TABLE sqlite_sequence (name, seq);
