-- Bound telemetry growth. 000034 partitioned raw_register_reading by
-- observed_at specifically so retention could be "a cheap DROP TABLE instead
-- of a slow DELETE", but nothing ever dropped anything: the partitions only
-- run to 2027-03 and every row stayed forever.
--
-- apply_retention does both halves, because monthly partitions cannot express
-- a sub-month retention window on their own:
--   1. DROP any partition whose whole range is already older than the cutoff
--      -- instant, no dead tuples, reclaims the files.
--   2. DELETE the stragglers from the partition the cutoff falls inside (and
--      from the DEFAULT partition). With a short window this is one run's
--      worth of rows, not a table scan's worth.
--
-- Deliberately NOT touched:
--   - audit_log: append-only by trigger, and it is the compliance record.
--     Its growth is a separate policy decision, not telemetry retention.
--   - device / plant / Register Metadata: inventory, not time series.

CREATE FUNCTION telemetry.apply_retention(p_keep interval)
RETURNS TABLE (dropped_partitions integer, deleted_readings bigint, deleted_batches bigint)
LANGUAGE plpgsql
AS $$
DECLARE
    cutoff timestamptz;
    part record;
    upper_bound timestamptz;
    removed bigint;
BEGIN
    IF p_keep IS NULL OR p_keep <= interval '0' THEN
        RAISE EXCEPTION 'retention window must be positive, got %', p_keep;
    END IF;
    cutoff := now() - p_keep;
    dropped_partitions := 0;
    deleted_readings := 0;
    deleted_batches := 0;

    -- Step 1: whole partitions that end at or before the cutoff.
    -- The bound is read from the catalog rather than parsed out of the
    -- partition name on purpose: 000034 created
    -- raw_register_reading_2027_02 covering 2026-12-01..2027-03-01, so the
    -- name does not reliably describe the range. The LIKE filter skips the
    -- DEFAULT partition, whose bound expression is just 'DEFAULT'.
    FOR part IN
        SELECT child.oid::regclass AS relation,
               (regexp_match(pg_get_expr(child.relpartbound, child.oid),
                             'TO \(''([^'']+)''\)'))[1] AS upper_text
        FROM pg_class child
        JOIN pg_inherits inh ON inh.inhrelid = child.oid
        WHERE inh.inhparent = 'telemetry.raw_register_reading'::regclass
          AND pg_get_expr(child.relpartbound, child.oid) LIKE 'FOR VALUES FROM %'
    LOOP
        CONTINUE WHEN part.upper_text IS NULL;
        upper_bound := part.upper_text::timestamptz;
        IF upper_bound <= cutoff THEN
            EXECUTE format('DROP TABLE %s', part.relation);
            dropped_partitions := dropped_partitions + 1;
        END IF;
    END LOOP;

    -- Step 2: rows older than the cutoff inside partitions that are still
    -- partly in range.
    DELETE FROM telemetry.raw_register_reading WHERE observed_at < cutoff;
    GET DIAGNOSTICS removed = ROW_COUNT;
    deleted_readings := removed;

    -- Step 3: ingest batches nobody references any more. Matched on "has no
    -- surviving readings" rather than on received_at alone: a batch received
    -- minutes ago can carry readings observed days earlier (a Middleware
    -- working off a backlog does exactly this), and the readings' FK would
    -- abort the whole run.
    DELETE FROM telemetry.telemetry_ingest_batch batch
    WHERE batch.received_at < cutoff
      AND NOT EXISTS (
          SELECT 1 FROM telemetry.raw_register_reading reading
          WHERE reading.organization_id = batch.organization_id
            AND reading.ingest_batch_id = batch.id
      );
    GET DIAGNOSTICS removed = ROW_COUNT;
    deleted_batches := removed;

    RETURN NEXT;
END;
$$;

-- ponytail: partitions still stop at 2027-03, after which everything lands in
-- raw_register_reading_default and step 1 stops reclaiming anything (step 2
-- still bounds the size, just with DELETE instead of DROP). Add monthly
-- pre-creation before then, or switch to daily partitions if the retention
-- window stays this short.
