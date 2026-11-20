-- 004: strain + breeder/manufacturer are one unit ("Lemon Sorbet" by Bathera and by another breeder
-- are two different strains). The data step in breeders.go fills the new columns from existing entries.

CREATE TABLE breeders (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    name_norm  text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX breeders_trgm_idx ON breeders USING gin (name_norm gin_trgm_ops);

-- NULL = "ohne Angabe". A strain name is unique per breeder, NULL counts as one value.
ALTER TABLE strains ADD COLUMN breeder_id uuid REFERENCES breeders(id);
ALTER TABLE strains DROP CONSTRAINT strains_name_norm_key;
ALTER TABLE strains ADD CONSTRAINT strains_variant_key UNIQUE NULLS NOT DISTINCT (name_norm, breeder_id);
CREATE INDEX strains_breeder_idx ON strains(breeder_id);

-- "Lemon Sorbet · Bathera": how a strain is named in reports, notices and the moderation log.
CREATE FUNCTION strain_label(sid uuid) RETURNS text LANGUAGE sql STABLE AS $$
    SELECT s.name || COALESCE(' · ' || b.name, '') FROM strains s LEFT JOIN breeders b ON b.id=s.breeder_id WHERE s.id=sid
$$;
