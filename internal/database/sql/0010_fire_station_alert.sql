-- Fire stations alerted for an incident. The incident itself lives in Automax.

CREATE TABLE fire_station_alert (
    id UUID PRIMARY KEY,
    incident_id UUID,
    service_point_id UUID NOT NULL REFERENCES service_point (id) ON DELETE RESTRICT,
    department_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_fire_station_alert_incident_station
    ON fire_station_alert (incident_id, service_point_id);

CREATE UNIQUE INDEX uq_fire_station_alert_station_without_incident
    ON fire_station_alert (service_point_id)
    WHERE incident_id IS NULL;

CREATE INDEX idx_fire_station_alert_incident
    ON fire_station_alert (incident_id);
