CREATE TABLE IF NOT EXISTS "repo_seq" (
  "seq" integer primary key autoincrement,
  "did" varchar not null,
  "eventType" varchar not null,
  "event" blob not null,
  "invalidated" int2 default 0 not null,
  "sequencedAt" varchar not null
);

CREATE INDEX IF NOT EXISTS "repo_seq_did_idx" ON "repo_seq" ("did");
CREATE INDEX IF NOT EXISTS "repo_seq_event_type_idx" ON "repo_seq" ("eventType");
CREATE INDEX IF NOT EXISTS "repo_seq_sequenced_at_index" ON "repo_seq" ("sequencedAt");
