package data

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// drainTimeout gives in-flight requests time to finish after a client swap.
const drainTimeout = 30 * time.Second

// PgPool is a stable handle whose underlying pool can be replaced after a
// configuration update. It also implements sqlc's DBTX interface.
type PgPool struct{ p atomic.Pointer[pgxpool.Pool] }

func NewPgPool(pool *pgxpool.Pool) *PgPool {
	h := &PgPool{}
	h.p.Store(pool)
	return h
}

func (h *PgPool) Pool() *pgxpool.Pool                   { return h.p.Load() }
func (h *PgPool) Swap(pool *pgxpool.Pool) *pgxpool.Pool { return h.p.Swap(pool) }

func (h *PgPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return h.p.Load().Exec(ctx, sql, args...)
}

func (h *PgPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return h.p.Load().Query(ctx, sql, args...)
}

func (h *PgPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return h.p.Load().QueryRow(ctx, sql, args...)
}

func (h *PgPool) Stat() *pgxpool.Stat     { return h.p.Load().Stat() }
func (h *PgPool) Config() *pgxpool.Config { return h.p.Load().Config() }
