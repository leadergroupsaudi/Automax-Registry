-- DSP-01. Dispatch tables live in this database.
-- thing_id, user_id, incident_id, and department_id are plain references.
-- They are not foreign keys, because those rows live in another service.
-- A resource is the same asset as a registry thing: thing_id is unique, and
-- the resource write copies type, station, and status onto that things row.

CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE resource_type (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    department_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    acceptance_timeout_seconds INT NOT NULL DEFAULT 120,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_resource_type_timeout CHECK (acceptance_timeout_seconds >= 1)
);

CREATE INDEX idx_resource_type_department_id ON resource_type (department_id);

CREATE TABLE service_point (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    location geography(Point, 4326) NOT NULL,
    department_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_service_point_department_id ON service_point (department_id);

CREATE TABLE shift (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    department_id UUID NOT NULL,
    name VARCHAR(100) NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    days_of_week VARCHAR(20) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_shift_times_differ CHECK (start_time <> end_time)
);

CREATE INDEX idx_shift_department_id ON shift (department_id);

CREATE TABLE resource (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thing_id UUID NOT NULL,
    resource_type_id UUID NOT NULL,
    service_point_id UUID,
    status VARCHAR(20) NOT NULL DEFAULT 'available',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_resource_thing UNIQUE (thing_id),
    CONSTRAINT fk_resource_type
        FOREIGN KEY (resource_type_id) REFERENCES resource_type (id) ON DELETE RESTRICT,
    CONSTRAINT fk_resource_service_point
        FOREIGN KEY (service_point_id) REFERENCES service_point (id) ON DELETE RESTRICT,
    CONSTRAINT ck_resource_status CHECK (status IN ('available', 'offered', 'dispatched', 'maintenance'))
);

CREATE INDEX idx_resource_resource_type_id ON resource (resource_type_id);
CREATE INDEX idx_resource_service_point_id ON resource (service_point_id);
CREATE INDEX idx_resource_status ON resource (status);

ALTER TABLE things
    ADD COLUMN resource_type_id UUID,
    ADD COLUMN service_point_id UUID,
    ADD COLUMN resource_status VARCHAR(20),
    ADD CONSTRAINT ck_things_resource_status CHECK (
        resource_status IS NULL
        OR resource_status IN ('available', 'offered', 'dispatched', 'maintenance')
    );

CREATE TABLE resource_shift_assignment (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_id UUID NOT NULL,
    user_id UUID NOT NULL,
    shift_id UUID,
    start_at TIMESTAMPTZ,
    end_at TIMESTAMPTZ,
    status VARCHAR(20) NOT NULL DEFAULT 'scheduled',
    CONSTRAINT fk_resource_shift_assignment_resource
        FOREIGN KEY (resource_id) REFERENCES resource (id) ON DELETE RESTRICT,
    CONSTRAINT fk_resource_shift_assignment_shift
        FOREIGN KEY (shift_id) REFERENCES shift (id) ON DELETE RESTRICT,
    CONSTRAINT ck_resource_shift_assignment_status CHECK (status IN ('scheduled', 'active', 'completed', 'cancelled')),
    CONSTRAINT ck_resource_shift_assignment_window CHECK (start_at IS NULL OR end_at IS NULL OR end_at > start_at)
);

CREATE INDEX idx_resource_shift_assignment_resource_id ON resource_shift_assignment (resource_id);
CREATE INDEX idx_resource_shift_assignment_status ON resource_shift_assignment (status);

CREATE TABLE dispatch_assignment (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID NOT NULL,
    resource_id UUID NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'master',
    status VARCHAR(20) NOT NULL DEFAULT 'offered',
    offered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    CONSTRAINT fk_dispatch_assignment_resource
        FOREIGN KEY (resource_id) REFERENCES resource (id) ON DELETE RESTRICT,
    CONSTRAINT ck_dispatch_assignment_role CHECK (role IN ('master', 'supporting')),
    CONSTRAINT ck_dispatch_assignment_status CHECK (status IN ('offered', 'accepted', 'rejected', 'expired', 'completed', 'cancelled'))
);

CREATE INDEX idx_dispatch_assignment_resource_id ON dispatch_assignment (resource_id);
CREATE INDEX idx_dispatch_assignment_incident_id ON dispatch_assignment (incident_id);

CREATE TABLE dispatch_ticket_projection (
    incident_id UUID PRIMARY KEY,
    classification_name VARCHAR(255),
    address TEXT,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    current_workflow_state_name VARCHAR(255),
    entered_dispatch_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL
);
