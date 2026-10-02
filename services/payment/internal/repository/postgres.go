package repository

import (
	"context"
	"database/sql"
	"errors"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/payment/internal/domain"
)

type Postgres struct {
	db *sql.DB
	t  *telemetry.Telemetry
}

func NewPostgres(db *sql.DB, t *telemetry.Telemetry) *Postgres { return &Postgres{db, t} }
func (r *Postgres) Save(ctx context.Context, p domain.Payment) (saved domain.Payment, created bool, err error) {
	ctx, finish := r.t.DB(ctx, "INSERT", "payments")
	defer func() { finish(err) }()
	err = r.db.QueryRowContext(ctx, `INSERT INTO payments(id,order_id,total,status,created_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(order_id) DO NOTHING RETURNING id,order_id,total,status,created_at`, p.ID, p.OrderID, p.Total, p.Status, p.CreatedAt).Scan(&saved.ID, &saved.OrderID, &saved.Total, &saved.Status, &saved.CreatedAt)
	if err == nil {
		return saved, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return saved, false, err
	}
	// Idempotency на order_id: повторный checkout не создаёт второе списание.
	ctx2, done := r.t.DB(ctx, "SELECT", "payments")
	defer func() { done(err) }()
	err = r.db.QueryRowContext(ctx2, `SELECT id,order_id,total,status,created_at FROM payments WHERE order_id=$1`, p.OrderID).Scan(&saved.ID, &saved.OrderID, &saved.Total, &saved.Status, &saved.CreatedAt)
	if err == nil && saved.Total != p.Total {
		return saved, false, domain.ErrConflict
	}
	return saved, false, err
}
func (r *Postgres) Get(ctx context.Context, id string) (p domain.Payment, err error) {
	ctx, finish := r.t.DB(ctx, "SELECT", "payments")
	defer func() { finish(err) }()
	err = r.db.QueryRowContext(ctx, `SELECT id,order_id,total,status,created_at FROM payments WHERE id=$1`, id).Scan(&p.ID, &p.OrderID, &p.Total, &p.Status, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = domain.ErrNotFound
	}
	return p, err
}
