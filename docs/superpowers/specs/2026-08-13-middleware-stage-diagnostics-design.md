# Middleware Stage Diagnostics Design

## Goal

Make a patch stage safe to run at active sites, show byte-accurate download progress, and return a support-ready cause instead of a generic HTTP 504.

## Design

The Middleware owns a process-local maintenance gate. A stage command closes the gate, waits for any poll sweep already in progress, downloads and validates the patch, then opens the gate in a deferred cleanup path. New poll sweeps skip while the gate is closed. Existing telemetry remains in SQLite; it is never discarded or force-cancelled.

The realtime protocol gains `command.progress`. Stage emits phases `maintenance`, `download`, `verify`, `stage`, and `resume`, with downloaded/total byte counts. The Platform hub forwards progress to the matching update job. A job item exposes phase, percentage, bytes, elapsed time, estimated remaining time, structured error code, detail, correlation ID, and retryability.

Errors use a safe `MiddlewareStageError` payload. HTTP responses preserve the appropriate status but include JSON diagnostics. The web page renders phase-specific text and a copyable support payload rather than reducing all failures to `Gateway time-out 504`.

## Constraints

- Do not stop the Middleware OS service before stage completes.
- Do not interrupt an in-flight Modbus request or delete queued SQLite telemetry.
- Always leave maintenance mode on stage success, validation failure, network failure, or websocket write failure.
- Progress derives from bytes read and the patch's known size; ETA is only shown after a positive measured transfer rate exists.
