-- The incident a unit is currently attached to. Plain UUID: incidents live in another service.

ALTER TABLE resource ADD COLUMN incident_id UUID;

CREATE INDEX idx_resource_incident_id ON resource (incident_id);
