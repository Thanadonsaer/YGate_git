-- API Polling (whether a gateway should send telemetry over the REST ingest
-- API on its own interval, see modbus-api-middleware's Worker.SendOnce) was
-- previously only toggleable from the middleware's own local web UI. Add it
-- to middleware_client so it can be set from the YGate web Middleware
-- Gateway page and pushed down as part of every config.snapshot, same as
-- poll_interval_seconds.
ALTER TABLE middleware_client
    ADD COLUMN api_polling_enabled boolean NOT NULL DEFAULT false;
