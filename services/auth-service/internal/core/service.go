package core

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"ygate/auth-service/internal/database/dbgen"
)

type Service struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, queries: dbgen.New(pool)}
}
