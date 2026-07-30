package dbgen

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const insertRawRegisterReading = `-- name: InsertRawRegisterReading :one
INSERT INTO raw_register_reading (
    id, organization_id, plant_id, device_id, middleware_client_id, ingest_batch_id,
    gateway_id, external_key, observed_at, register_address_map, parameter_count
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (middleware_client_id, external_key) DO NOTHING
RETURNING id
`

type InsertRawRegisterReadingParams struct {
	ID                 pgtype.UUID
	OrganizationID     pgtype.UUID
	PlantID            pgtype.UUID
	DeviceID           pgtype.UUID
	MiddlewareClientID pgtype.UUID
	IngestBatchID      pgtype.UUID
	GatewayID          string
	ExternalKey        string
	ObservedAt         pgtype.Timestamptz
	RegisterAddressMap []byte
	ParameterCount     int32
}

func (q *Queries) InsertRawRegisterReading(ctx context.Context, arg InsertRawRegisterReadingParams) (pgtype.UUID, error) {
	row := q.db.QueryRow(ctx, insertRawRegisterReading,
		arg.ID,
		arg.OrganizationID,
		arg.PlantID,
		arg.DeviceID,
		arg.MiddlewareClientID,
		arg.IngestBatchID,
		arg.GatewayID,
		arg.ExternalKey,
		arg.ObservedAt,
		arg.RegisterAddressMap,
		arg.ParameterCount,
	)
	var id pgtype.UUID
	err := row.Scan(&id)
	return id, err
}
