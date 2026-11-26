-- A dispatch log can also record a unit dropped on a fire station, or an
-- incident assigned to a fire station. Those rows have no assignment.

ALTER TABLE dispatch_log
    ALTER COLUMN assignment_id DROP NOT NULL,
    ALTER COLUMN incident_id DROP NOT NULL,
    ALTER COLUMN resource_id DROP NOT NULL,
    ALTER COLUMN role DROP NOT NULL;

ALTER TABLE dispatch_log DROP CONSTRAINT ck_dispatch_log_action;
ALTER TABLE dispatch_log ADD CONSTRAINT ck_dispatch_log_action CHECK (
    action IN (
        'created', 'status_changed', 'updated', 'deleted',
        'station_assigned', 'station_unassigned', 'incident_assigned'
    )
);

ALTER TABLE dispatch_log
    ADD COLUMN fire_station_id UUID,
    ADD COLUMN fire_station_name VARCHAR(255);

CREATE INDEX idx_dispatch_log_fire_station_id ON dispatch_log (fire_station_id, created_at);
