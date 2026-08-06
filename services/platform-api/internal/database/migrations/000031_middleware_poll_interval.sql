-- Poll interval was collected per Device but never actually used: neither
-- buildConfigSnapshot nor the wire protocol ever sent it, and
-- modbus-api-middleware has no per-connection interval concept at all --
-- polling cadence there is one gateway-wide SendIntervalSeconds value (see
-- internal/delivery/worker.go), configured locally on the middleware's own
-- web UI. Move the setting to where it's real: middleware_client, pushed to
-- the gateway's GatewayConfig as part of every Push Config instead of being
-- a dead per-device field that implied a granularity the system never had.
ALTER TABLE device
    DROP CONSTRAINT device_poll_interval_range,
    DROP COLUMN poll_interval_seconds;

ALTER TABLE middleware_client
    ADD COLUMN poll_interval_seconds integer NOT NULL DEFAULT 60,
    ADD CONSTRAINT middleware_client_poll_interval_range CHECK (poll_interval_seconds BETWEEN 5 AND 3600);
