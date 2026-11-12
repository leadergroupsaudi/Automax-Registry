-- Unit type, unit, and fire station fields used by Dispatch Management.
-- Existing rows keep working: new text columns default to empty, flags default to active.

ALTER TABLE resource_type
    ADD COLUMN code VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN alt_name VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN min_speed INT NOT NULL DEFAULT 0,
    ADD COLUMN max_speed INT NOT NULL DEFAULT 0,
    ADD COLUMN min_weight INT NOT NULL DEFAULT 0,
    ADD COLUMN max_weight INT NOT NULL DEFAULT 0,
    ADD COLUMN active_till_date DATE,
    ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN icon VARCHAR(2048) NOT NULL DEFAULT '',
    ADD COLUMN colors JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE resource_type
    ADD CONSTRAINT ck_resource_type_speed CHECK (min_speed >= 0 AND max_speed >= 0 AND max_speed >= min_speed),
    ADD CONSTRAINT ck_resource_type_weight CHECK (min_weight >= 0 AND max_weight >= 0 AND max_weight >= min_weight);

CREATE UNIQUE INDEX uq_resource_type_department_code
    ON resource_type (department_id, code)
    WHERE code <> '';

ALTER TABLE service_point
    ADD COLUMN alt_name VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN radio_id VARCHAR(100) NOT NULL DEFAULT '',
    ADD COLUMN location_name VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN active_till_date DATE,
    ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN icon VARCHAR(2048) NOT NULL DEFAULT '';

ALTER TABLE resource
    ADD COLUMN alternate_name VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN plate_number VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN location_name VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN classification JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN description TEXT NOT NULL DEFAULT '',
    ADD COLUMN radio_id VARCHAR(100) NOT NULL DEFAULT '',
    ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN active_till_date DATE,
    ADD COLUMN call_code VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE resource_shift_assignment
    ADD COLUMN partner_user_id UUID;
