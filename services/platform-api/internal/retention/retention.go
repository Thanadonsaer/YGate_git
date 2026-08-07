// Package retention runs the telemetry retention window on a ticker.
//
// The policy itself lives in SQL (telemetry.apply_retention, migration
// 000040) so it can be run by hand or from a DBA's cron without going
// through the service; this package only decides when to call it.
package retention

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultKeep is the fallback if Run is called with a non-positive window;
// the configured default lives with the other env defaults in internal/config.
const defaultKeep = 7 * 24 * time.Hour

const runTimeout = 10 * time.Minute

// Run applies the retention window immediately, then every interval until ctx
// is done. Call it as `go retention.Run(...)`.
//
// A failed run is logged and retried on the next tick rather than being fatal:
// retention falling behind is a disk-space problem, not a reason to take
// ingestion down.
func Run(ctx context.Context, pool *pgxpool.Pool, keep, interval time.Duration) {
	if keep <= 0 {
		keep = defaultKeep
	}
	if interval <= 0 {
		interval = time.Hour
	}
	log.Printf("telemetry retention: keeping %s, sweeping every %s", keep, interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		applyOnce(ctx, pool, keep)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func applyOnce(ctx context.Context, pool *pgxpool.Pool, keep time.Duration) {
	runCtx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()
	var droppedPartitions int32
	var deletedReadings, deletedBatches int64
	if err := pool.QueryRow(runCtx,
		`SELECT dropped_partitions, deleted_readings, deleted_batches FROM telemetry.apply_retention($1)`,
		keep,
	).Scan(&droppedPartitions, &deletedReadings, &deletedBatches); err != nil {
		log.Printf("telemetry retention: sweep failed: %v", err)
		return
	}
	if droppedPartitions == 0 && deletedReadings == 0 && deletedBatches == 0 {
		return
	}
	log.Printf("telemetry retention: dropped %d partition(s), deleted %d reading(s) and %d ingest batch(es) older than %s",
		droppedPartitions, deletedReadings, deletedBatches, keep)
}
