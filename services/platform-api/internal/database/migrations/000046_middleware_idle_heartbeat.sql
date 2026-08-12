-- Per-gateway control over the idle heartbeat.
--
-- The Middleware stops enqueueing a reading whose register values have not
-- moved since the last one it stored, and sends one anyway every so often so
-- the platform can still tell an idle device from a dead one. That interval
-- was a constant compiled into the gateway; it belongs next to
-- poll_interval_seconds, edited on the Middleware Gateways page and pushed
-- down with every config snapshot, exactly as 000031 did for the poll interval.
--
-- Bounds: at least a minute (anything shorter defeats the point of skipping
-- unchanged readings), at most a day (so a typo cannot leave a dead device
-- looking alive for a week). 1800s / 30 min matches the gateway's own
-- app.DefaultIdleHeartbeat, so existing gateways see no behaviour change until
-- someone edits it.

ALTER TABLE auth.middleware_client
    ADD COLUMN idle_heartbeat_seconds integer NOT NULL DEFAULT 1800,
    ADD CONSTRAINT middleware_client_idle_heartbeat_range
        CHECK (idle_heartbeat_seconds BETWEEN 60 AND 86400);
