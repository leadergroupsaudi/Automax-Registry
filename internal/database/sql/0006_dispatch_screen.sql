-- Fields the dispatch screens send that were not on the unit or fire station.

ALTER TABLE resource
    ADD COLUMN unit_code VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN location_id UUID,
    ADD COLUMN classification_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN gprs VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN sms VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN sms_vip VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN sim VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN driver_user_id UUID,
    ADD COLUMN shift_id UUID,
    ADD COLUMN partner_user_id UUID;

CREATE UNIQUE INDEX uq_resource_unit_code
    ON resource (unit_code)
    WHERE unit_code <> '';

ALTER TABLE resource
    ADD CONSTRAINT fk_resource_shift FOREIGN KEY (shift_id) REFERENCES shift (id);

ALTER TABLE service_point
    ADD COLUMN location_id UUID;

CREATE UNIQUE INDEX uq_service_point_department_radio
    ON service_point (department_id, radio_id)
    WHERE radio_id <> '';

CREATE TABLE dispatch_icon (
    id UUID PRIMARY KEY,
    content_type VARCHAR(64) NOT NULL,
    body BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
