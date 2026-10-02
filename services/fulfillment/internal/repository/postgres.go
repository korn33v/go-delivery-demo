package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"godelivery/contracts/events"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/fulfillment/internal/domain"
	"time"
)

type Postgres struct {
	db *sql.DB
	t  *telemetry.Telemetry
}

func NewPostgres(db *sql.DB, t *telemetry.Telemetry) *Postgres { return &Postgres{db, t} }
func (r *Postgres) Accept(ctx context.Context, inputEvent string, f domain.Fulfillment, eventID string) (created bool, err error) {
	ctx, finish := r.t.DB(ctx, "INSERT", "fulfillments")
	defer func() { finish(err) }()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO processed_events(event_id) VALUES($1) ON CONFLICT DO NOTHING`, inputEvent)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, tx.Commit()
	}
	headers, err := json.Marshal(f.TraceHeaders)
	if err != nil {
		return false, err
	}
	res, err = tx.ExecContext(ctx, `INSERT INTO fulfillments(id,order_id,status,sequence,trace_headers,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(order_id) DO NOTHING`, f.ID, f.OrderID, f.Status, f.Sequence, headers, f.CreatedAt, f.UpdatedAt)
	if err != nil {
		return false, err
	}
	n, _ = res.RowsAffected()
	if n > 0 {
		if err = r.enqueue(ctx, tx, eventID, f); err != nil {
			return false, err
		}
		created = true
	}
	return created, tx.Commit()
}
func (r *Postgres) Get(ctx context.Context, id string) (f domain.Fulfillment, err error) {
	ctx, finish := r.t.DB(ctx, "SELECT", "fulfillments")
	defer func() { finish(err) }()
	var headers []byte
	err = r.db.QueryRowContext(ctx, `SELECT id,order_id,status,sequence,trace_headers,created_at,updated_at FROM fulfillments WHERE id=$1`, id).Scan(&f.ID, &f.OrderID, &f.Status, &f.Sequence, &headers, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return f, domain.ErrNotFound
	}
	if err != nil {
		return f, err
	}
	err = json.Unmarshal(headers, &f.TraceHeaders)
	return f, err
}
func (r *Postgres) Advance(ctx context.Context, id, eventID string) (f domain.Fulfillment, err error) {
	ctx, finish := r.t.DB(ctx, "UPDATE", "fulfillments")
	defer func() { finish(err) }()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return f, err
	}
	defer tx.Rollback()
	var headers []byte
	// FOR UPDATE сериализует concurrent advance: нет двух одинаковых sequence.
	err = tx.QueryRowContext(ctx, `SELECT id,order_id,status,sequence,trace_headers,created_at FROM fulfillments WHERE id=$1 FOR UPDATE`, id).Scan(&f.ID, &f.OrderID, &f.Status, &f.Sequence, &headers, &f.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return f, domain.ErrNotFound
	}
	if err != nil {
		return f, err
	}
	f.Status, err = domain.Next(f.Status)
	if err != nil {
		return f, err
	}
	f.Sequence++
	f.UpdatedAt = time.Now().UTC()
	if err = json.Unmarshal(headers, &f.TraceHeaders); err != nil {
		return f, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE fulfillments SET status=$2,sequence=$3,updated_at=$4 WHERE id=$1`, id, f.Status, f.Sequence, f.UpdatedAt)
	if err != nil {
		return f, err
	}
	if err = r.enqueue(ctx, tx, eventID, f); err != nil {
		return f, err
	}
	return f, tx.Commit()
}
func (r *Postgres) enqueue(ctx context.Context, tx *sql.Tx, eventID string, f domain.Fulfillment) (err error) {
	ctx, finish := r.t.DB(ctx, "INSERT", "outbox")
	defer func() { finish(err) }()
	e := events.StatusChanged{EventID: eventID, EventType: events.StatusTopic, EventVersion: 1, OccurredAt: time.Now().UTC(), OrderID: f.OrderID, FulfillmentID: f.ID, Status: f.Status, Sequence: f.Sequence}
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	headers, err := json.Marshal(telemetry.TraceHeaders(ctx))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox(event_id,topic,message_key,payload,trace_headers) VALUES($1,$2,$3,$4,$5)`, eventID, events.StatusTopic, f.OrderID, payload, headers)
	return err
}
