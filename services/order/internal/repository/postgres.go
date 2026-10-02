package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"godelivery/contracts/events"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/order/internal/domain"
	"time"
)

type Postgres struct {
	db *sql.DB
	t  *telemetry.Telemetry
}

func NewPostgres(db *sql.DB, t *telemetry.Telemetry) *Postgres { return &Postgres{db, t} }
func (r *Postgres) Create(ctx context.Context, o domain.Order) (err error) {
	ctx, finish := r.t.DB(ctx, "INSERT", "orders")
	defer func() { finish(err) }()
	items, err := json.Marshal(o.Items)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO orders(id,customer_id,restaurant_id,items,total,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, o.ID, o.CustomerID, o.RestaurantID, items, o.Total, o.Status, o.CreatedAt, o.UpdatedAt)
	return err
}
func (r *Postgres) Get(ctx context.Context, id string) (o domain.Order, err error) {
	ctx, finish := r.t.DB(ctx, "SELECT", "orders")
	defer func() { finish(err) }()
	var items []byte
	err = r.db.QueryRowContext(ctx, `SELECT id,customer_id,restaurant_id,items,total,status,payment_id,sequence,created_at,updated_at FROM orders WHERE id=$1`, id).Scan(&o.ID, &o.CustomerID, &o.RestaurantID, &items, &o.Total, &o.Status, &o.PaymentID, &o.Sequence, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return o, domain.ErrNotFound
	}
	if err != nil {
		return o, err
	}
	err = json.Unmarshal(items, &o.Items)
	return o, err
}
func (r *Postgres) MarkPaid(ctx context.Context, id, paymentID, eventID string) (err error) {
	ctx, finish := r.t.DB(ctx, "UPDATE", "orders")
	defer func() { finish(err) }()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var total int64
	err = tx.QueryRowContext(ctx, `UPDATE orders SET status='Paid',payment_id=$2,updated_at=now() WHERE id=$1 AND status='Created' RETURNING total`, id, paymentID).Scan(&total)
	if err != nil {
		return err
	}
	paid := events.OrderPaid{EventID: eventID, EventType: events.PaidTopic, EventVersion: 1, OccurredAt: time.Now().UTC(), OrderID: id, PaymentID: paymentID, Total: total}
	// tx передаётся явно: статус и transport DTO фиксируются одним ACID commit.
	if err = r.enqueue(ctx, tx, eventID, events.PaidTopic, id, paid); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Postgres) MarkFailed(ctx context.Context, id, status, paymentID string) (err error) {
	ctx, finish := r.t.DB(ctx, "UPDATE", "orders")
	defer func() { finish(err) }()
	_, err = r.db.ExecContext(ctx, `UPDATE orders SET status=$2,payment_id=$3,updated_at=now() WHERE id=$1 AND status='Created'`, id, status, paymentID)
	return err
}
func (r *Postgres) ApplyStatus(ctx context.Context, eventID, orderID string, seq int, status string) (changed bool, o domain.Order, err error) {
	ctx, finish := r.t.DB(ctx, "UPDATE", "orders")
	defer func() { finish(err) }()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, o, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO processed_events(event_id) VALUES($1) ON CONFLICT DO NOTHING`, eventID)
	if err != nil {
		return false, o, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, o, tx.Commit()
	}
	err = tx.QueryRowContext(ctx, `SELECT id,status,sequence,created_at FROM orders WHERE id=$1 FOR UPDATE`, orderID).Scan(&o.ID, &o.Status, &o.Sequence, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, o, domain.ErrNotFound
	}
	if err != nil {
		return false, o, err
	}
	if err = o.CanApply(status, seq); err != nil {
		return false, o, err
	}
	if seq > o.Sequence {
		_, err = tx.ExecContext(ctx, `UPDATE orders SET status=$2,sequence=$3,updated_at=now() WHERE id=$1`, orderID, status, seq)
		if err != nil {
			return false, o, err
		}
		changed = true
		o.Status = status
		o.Sequence = seq
	}
	err = tx.Commit()
	return changed, o, err
}
func (r *Postgres) Counts(ctx context.Context) (result map[string]int64, err error) {
	ctx, finish := r.t.DB(ctx, "SELECT", "orders")
	defer func() { finish(err) }()
	rows, err := r.db.QueryContext(ctx, `SELECT status,count(*) FROM orders GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result = map[string]int64{}
	for rows.Next() {
		var status string
		var n int64
		if err = rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		result[status] = n
	}
	return result, rows.Err()
}
func (r *Postgres) enqueue(ctx context.Context, tx *sql.Tx, eventID, topic, key string, value any) (err error) {
	ctx, finish := r.t.DB(ctx, "INSERT", "outbox")
	defer func() { finish(err) }()
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	headers, err := json.Marshal(telemetry.TraceHeaders(ctx))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox(event_id,topic,message_key,payload,trace_headers) VALUES($1,$2,$3,$4,$5)`, eventID, topic, key, payload, headers)
	return err
}
