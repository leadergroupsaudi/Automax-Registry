-- Drivers registered from Automax. unique_id is the identifier sent to Caguar.

CREATE TABLE driver (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    unique_id VARCHAR(255) NOT NULL,
    pending_action VARCHAR(16) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_driver_pending_action CHECK (pending_action IN ('', 'create', 'update'))
);

CREATE UNIQUE INDEX uq_driver_unique_id ON driver (unique_id);
