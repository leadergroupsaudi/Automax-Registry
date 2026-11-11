-- Names are case-sensitive. The same spelling cannot be created twice
-- in one department. A different spelling, or the same spelling in another
-- department, is a different record. The id is always a new UUID.
-- Existing duplicates keep the oldest row's name. Later rows get a short id suffix
-- so this migration can finish on a database that already has repeated names.

UPDATE things t
SET name = left(t.name, 220) || ' [' || substr(t.id::text, 1, 8) || ']'
WHERE t.id IN (
    SELECT id FROM (
        SELECT id, row_number() OVER (PARTITION BY department_id, name ORDER BY created_at, id) AS rn
        FROM things
    ) d
    WHERE rn > 1
);

UPDATE groups g
SET name = left(g.name, 220) || ' [' || substr(g.id::text, 1, 8) || ']'
WHERE g.id IN (
    SELECT id FROM (
        SELECT id, row_number() OVER (PARTITION BY department_id, name ORDER BY created_at, id) AS rn
        FROM groups
    ) d
    WHERE rn > 1
);

UPDATE channels c
SET name = left(c.name, 220) || ' [' || substr(c.id::text, 1, 8) || ']'
WHERE c.id IN (
    SELECT id FROM (
        SELECT id, row_number() OVER (PARTITION BY name ORDER BY created_at, id) AS rn
        FROM channels
    ) d
    WHERE rn > 1
);

UPDATE resource_type r
SET name = left(r.name, 220) || ' [' || substr(r.id::text, 1, 8) || ']'
WHERE r.id IN (
    SELECT id FROM (
        SELECT id, row_number() OVER (PARTITION BY department_id, name ORDER BY created_at, id) AS rn
        FROM resource_type
    ) d
    WHERE rn > 1
);

UPDATE service_point s
SET name = left(s.name, 220) || ' [' || substr(s.id::text, 1, 8) || ']'
WHERE s.id IN (
    SELECT id FROM (
        SELECT id, row_number() OVER (PARTITION BY department_id, name ORDER BY created_at, id) AS rn
        FROM service_point
    ) d
    WHERE rn > 1
);

UPDATE shift s
SET name = left(s.name, 80) || ' [' || substr(s.id::text, 1, 8) || ']'
WHERE s.id IN (
    SELECT id FROM (
        SELECT id, row_number() OVER (PARTITION BY department_id, name ORDER BY created_at, id) AS rn
        FROM shift
    ) d
    WHERE rn > 1
);

ALTER TABLE things
    ADD CONSTRAINT uq_things_department_name UNIQUE (department_id, name);

ALTER TABLE groups
    ADD CONSTRAINT uq_groups_department_name UNIQUE (department_id, name);

ALTER TABLE channels
    ADD CONSTRAINT uq_channels_name UNIQUE (name);

ALTER TABLE resource_type
    ADD CONSTRAINT uq_resource_type_department_name UNIQUE (department_id, name);

ALTER TABLE service_point
    ADD CONSTRAINT uq_service_point_department_name UNIQUE (department_id, name);

ALTER TABLE shift
    ADD CONSTRAINT uq_shift_department_name UNIQUE (department_id, name);
