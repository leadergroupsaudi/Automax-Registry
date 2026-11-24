-- Who accepted a dispatch offer. The user lives in Automax-Backend, so this is
-- a plain id and the name sent with the accept call.

ALTER TABLE dispatch_log
    ADD COLUMN accepted_by_id UUID,
    ADD COLUMN accepted_by_name VARCHAR(255);
