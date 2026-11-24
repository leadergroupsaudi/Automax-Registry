-- History of dispatch assignment changes. The assignment row is the current
-- offer. This table keeps each offer, status change, and delete.

CREATE TABLE dispatch_log (
    id UUID PRIMARY KEY,
    assignment_id UUID NOT NULL,
    incident_id UUID NOT NULL,
    resource_id UUID NOT NULL,
    role VARCHAR(20) NOT NULL,
    action VARCHAR(20) NOT NULL,
    from_status VARCHAR(20),
    to_status VARCHAR(20),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_dispatch_log_role CHECK (role IN ('master', 'supporting')),
    CONSTRAINT ck_dispatch_log_action CHECK (action IN ('created', 'status_changed', 'updated', 'deleted')),
    CONSTRAINT ck_dispatch_log_from_status CHECK (
        from_status IS NULL
        OR from_status IN ('offered', 'accepted', 'rejected', 'expired', 'completed', 'cancelled')
    ),
    CONSTRAINT ck_dispatch_log_to_status CHECK (
        to_status IS NULL
        OR to_status IN ('offered', 'accepted', 'rejected', 'expired', 'completed', 'cancelled')
    )
);

CREATE INDEX idx_dispatch_log_incident_id ON dispatch_log (incident_id, created_at);
CREATE INDEX idx_dispatch_log_resource_id ON dispatch_log (resource_id, created_at);
CREATE INDEX idx_dispatch_log_assignment_id ON dispatch_log (assignment_id, created_at);
