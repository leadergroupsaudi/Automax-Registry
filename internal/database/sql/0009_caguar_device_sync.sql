-- Units saved while Caguar is down, and deletes Caguar has not confirmed yet.

CREATE TABLE caguar_device_sync (
    id UUID PRIMARY KEY,
    thing_id UUID NOT NULL,
    action VARCHAR(16) NOT NULL,
    unique_id VARCHAR(255) NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}',
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_caguar_device_sync_action CHECK (action IN ('create', 'update', 'delete'))
);

CREATE UNIQUE INDEX uq_caguar_device_sync_thing_action ON caguar_device_sync (thing_id, action);
