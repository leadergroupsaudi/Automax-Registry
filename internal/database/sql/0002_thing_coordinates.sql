-- AMAX-4332: latest known coordinates for a thing.
-- NULL means no live fix has been applied. 0 is a real coordinate and is not used as the empty value.

ALTER TABLE things ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION;
ALTER TABLE things ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION;
