-- Which resources were offered for an incident at a service point. resource_id is NULL
-- when the service point had no unit with a driver at the time.

CREATE TABLE incident_service_resource (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL,
    incident_id UUID NOT NULL,
    resource_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_isr_resource FOREIGN KEY (resource_id) REFERENCES resource (id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uq_isr_with_resource
    ON incident_service_resource (service_id, incident_id, resource_id)
    WHERE resource_id IS NOT NULL;

CREATE UNIQUE INDEX uq_isr_without_resource
    ON incident_service_resource (service_id, incident_id)
    WHERE resource_id IS NULL;

CREATE INDEX idx_isr_incident_id ON incident_service_resource (incident_id);
