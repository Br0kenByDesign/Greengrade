-- greengrade initial schema
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Accounts. No passwords, no e-mail in clear text: email_hmac = HMAC-SHA256(pepper, lower(email)).
CREATE TABLE users (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email_hmac         bytea UNIQUE,
    display_name       text NOT NULL,
    is_admin           boolean NOT NULL DEFAULT false,
    tokens_valid_after timestamptz NOT NULL DEFAULT date_trunc('second', now()),
    age_confirmed_at   timestamptz NOT NULL,
    consent_at         timestamptz NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_identities (
    provider   text NOT NULL,
    subject    text NOT NULL,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject),
    UNIQUE (user_id, provider)
);

-- Passkeys only contain public keys.
CREATE TABLE passkeys (
    id           bytea PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL,
    credential   jsonb NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);
CREATE INDEX passkeys_user_idx ON passkeys(user_id);

-- One-time use of magic links, signup tokens and proof-of-work challenges.
CREATE TABLE used_tokens (
    id         text PRIMARY KEY,
    expires_at timestamptz NOT NULL
);

CREATE TABLE rate_events (
    key text NOT NULL,
    at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX rate_events_key_idx ON rate_events(key, at);

-- Shared strain catalogue. name_norm is used for matching and suggestions.
CREATE TABLE strains (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    name_norm       text NOT NULL UNIQUE,
    pinned_photo_id uuid,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX strains_trgm_idx ON strains USING gin (name_norm gin_trgm_ops);

CREATE TABLE grows (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    strain_id            uuid REFERENCES strains(id),
    name                 text NOT NULL,
    location             text NOT NULL DEFAULT '',
    seed_type            text NOT NULL DEFAULT '',
    environment          text NOT NULL DEFAULT '',
    medium               text NOT NULL DEFAULT '',
    light_watts          integer,
    germinated_on        date,
    flowering_on         date,
    expected_flower_days integer,
    harvested_on         date,
    dry_days             integer,
    cure_weeks           integer,
    yield_grams          integer,
    notes                text NOT NULL DEFAULT '',
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX grows_user_idx ON grows(user_id);

CREATE TABLE grow_logs (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    grow_id    uuid NOT NULL REFERENCES grows(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    logged_on  date NOT NULL,
    text       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX grow_logs_grow_idx ON grow_logs(grow_id);

CREATE TABLE entries (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    strain_id  uuid NOT NULL REFERENCES strains(id),
    grow_id    uuid REFERENCES grows(id) ON DELETE SET NULL,
    source     text NOT NULL CHECK (source IN ('grow', 'pharmacy')),
    form       text NOT NULL CHECK (form IN ('flower', 'extract')),
    genetics   smallint NOT NULL DEFAULT 50 CHECK (genetics BETWEEN 0 AND 100),
    details    jsonb NOT NULL DEFAULT '{}',
    notes      text NOT NULL DEFAULT '',
    rebuy      boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX entries_user_idx ON entries(user_id);
CREATE INDEX entries_strain_idx ON entries(strain_id);

CREATE TABLE tastings (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entry_id     uuid NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tasted_on    date NOT NULL,
    method       text NOT NULL DEFAULT '',
    temperature  integer,
    onset_min    integer,
    duration_h   numeric(4,1),
    smell        smallint NOT NULL CHECK (smell BETWEEN 1 AND 10),
    taste        smallint NOT NULL CHECK (taste BETWEEN 1 AND 10),
    look         smallint NOT NULL CHECK (look BETWEEN 1 AND 10),
    effect       smallint NOT NULL CHECK (effect BETWEEN 1 AND 10),
    quality      smallint NOT NULL CHECK (quality BETWEEN 1 AND 10),
    aromas       text[] NOT NULL DEFAULT '{}',
    flavors      text[] NOT NULL DEFAULT '{}',
    effects      text[] NOT NULL DEFAULT '{}',
    side_effects text[] NOT NULL DEFAULT '{}',
    daytime      text[] NOT NULL DEFAULT '{}',
    note         text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tastings_entry_idx ON tastings(entry_id, tasted_on DESC, created_at DESC);

-- Files live on disk under DATA_DIR/photos, always re-encoded by the server.
CREATE TABLE photos (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entry_id   uuid NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    width      integer NOT NULL,
    height     integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX photos_entry_idx ON photos(entry_id);

ALTER TABLE strains ADD CONSTRAINT strains_pinned_photo_fk
    FOREIGN KEY (pinned_photo_id) REFERENCES photos(id) ON DELETE SET NULL;

-- Public ratings: only scores, optional comment and optional photo. Never who, where or how.
CREATE TABLE public_ratings (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    strain_id      uuid NOT NULL REFERENCES strains(id),
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entry_id       uuid NOT NULL UNIQUE REFERENCES entries(id) ON DELETE CASCADE,
    overall        numeric(3,1) NOT NULL,
    smell          smallint NOT NULL,
    taste          smallint NOT NULL,
    look           smallint NOT NULL,
    effect         smallint NOT NULL,
    quality        smallint NOT NULL,
    comment        text,
    comment_hidden boolean NOT NULL DEFAULT false,
    photo_id       uuid REFERENCES photos(id) ON DELETE SET NULL,
    photo_hidden   boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, strain_id)
);
CREATE INDEX public_ratings_strain_idx ON public_ratings(strain_id);

CREATE TABLE reports (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    public_rating_id uuid NOT NULL REFERENCES public_ratings(id) ON DELETE CASCADE,
    target           text NOT NULL CHECK (target IN ('comment', 'photo')),
    reason           text NOT NULL,
    details          text NOT NULL DEFAULT '',
    reporter_id      uuid REFERENCES users(id) ON DELETE SET NULL,
    status           text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'hidden', 'dismissed')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    resolved_at      timestamptz
);
CREATE INDEX reports_status_idx ON reports(status, created_at);

-- In-app messages (we cannot e-mail users - we do not know their address).
CREATE TABLE notices (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      text NOT NULL,
    body       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at    timestamptz
);
CREATE INDEX notices_user_idx ON notices(user_id, created_at DESC);
