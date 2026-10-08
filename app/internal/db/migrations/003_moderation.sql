-- 003: moderation history, content fingerprints, share bans

-- Fingerprints so that hidden content stays hidden when it is shared again.
ALTER TABLE photos ADD COLUMN content_hash bytea;
CREATE INDEX photos_hash_idx ON photos(content_hash);
ALTER TABLE public_ratings ADD COLUMN comment_fp bytea;
CREATE INDEX public_ratings_comment_fp_idx ON public_ratings(comment_fp);

-- scope '' = applies to everyone, otherwise the author's user id (short, generic comments)
CREATE TABLE moderation_blocks (
    kind        text NOT NULL CHECK (kind IN ('comment', 'photo')),
    fingerprint bytea NOT NULL,
    scope       text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, fingerprint, scope)
);

-- Accounts that may keep their private log but must not share publicly any more.
ALTER TABLE users ADD COLUMN share_banned_at timestamptz;
ALTER TABLE users ADD COLUMN share_ban_reason text NOT NULL DEFAULT '';

-- Reports survive when the reported rating is withdrawn, and can target strain names.
ALTER TABLE reports DROP CONSTRAINT reports_public_rating_id_fkey;
ALTER TABLE reports ALTER COLUMN public_rating_id DROP NOT NULL;
ALTER TABLE reports ADD CONSTRAINT reports_public_rating_id_fkey
    FOREIGN KEY (public_rating_id) REFERENCES public_ratings(id) ON DELETE SET NULL;
ALTER TABLE reports DROP CONSTRAINT reports_target_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_check CHECK (target IN ('comment', 'photo', 'name'));
ALTER TABLE reports ADD COLUMN strain_id uuid REFERENCES strains(id) ON DELETE SET NULL;
ALTER TABLE reports ADD COLUMN strain_name text NOT NULL DEFAULT '';
ALTER TABLE reports ADD COLUMN author_id uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE reports ADD COLUMN content text;      -- reported comment at report time
ALTER TABLE reports ADD COLUMN photo_id uuid REFERENCES photos(id) ON DELETE SET NULL;

UPDATE reports r SET strain_id = pr.strain_id, strain_name = s.name, author_id = pr.user_id,
       content = CASE WHEN r.target = 'comment' THEN pr.comment END,
       photo_id = CASE WHEN r.target = 'photo' THEN pr.photo_id END
FROM public_ratings pr JOIN strains s ON s.id = pr.strain_id
WHERE pr.id = r.public_rating_id;

-- Every moderation decision is logged. Content and the link to the account are removed
-- when the affected account is deleted; date, action and reason remain.
CREATE TABLE moderation_actions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    action         text NOT NULL,
    reason         text NOT NULL DEFAULT '',
    note           text NOT NULL DEFAULT '',
    strain_name    text NOT NULL DEFAULT '',
    content        text,
    target_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    actor_id       uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX moderation_actions_user_idx ON moderation_actions(target_user_id);
CREATE INDEX moderation_actions_time_idx ON moderation_actions(created_at DESC);
