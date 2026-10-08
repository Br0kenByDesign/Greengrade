-- Short codes as an alternative to the magic link (needed in installed iOS apps,
-- where links from mails open in Safari instead of the app).
CREATE TABLE login_codes (
    id         text PRIMARY KEY,           -- same id as the magic link token
    email_hmac bytea NOT NULL,
    code_hash  bytea NOT NULL,             -- HMAC of id and code, never the code itself
    attempts   smallint NOT NULL DEFAULT 0,
    expires_at timestamptz NOT NULL
);
