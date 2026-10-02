package repository

import (
	"context"
	"encoding/json"
	"godelivery/internal/platform/bus"
	"godelivery/internal/platform/telemetry"
	"time"
)

// Каждый outbox находится в БД своего сервиса; общий bus не читает SQL.
func (r *Postgres) RunOutbox(ctx context.Context, producer *bus.Producer) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.flush(ctx, producer); err != nil && ctx.Err() == nil {
				r.t.Logger.ErrorContext(ctx, "outbox will retry", "operation", "outbox.publish", "error", err.Error())
			}
		}
	}
}
func (r *Postgres) flush(ctx context.Context, producer *bus.Producer) (err error) {
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	queryCtx, finish := r.t.DB(queryCtx, "SELECT", "outbox")
	defer func() { finish(err) }()
	rows, err := r.db.QueryContext(queryCtx, `SELECT event_id,topic,message_key,payload,trace_headers FROM outbox WHERE published_at IS NULL ORDER BY created_at,event_id LIMIT 50`)
	if err != nil {
		return err
	}
	type pending struct {
		id, topic, key   string
		payload, headers []byte
	}
	var batch []pending
	for rows.Next() {
		var p pending
		if err = rows.Scan(&p.id, &p.topic, &p.key, &p.payload, &p.headers); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, p := range batch {
		var h map[string]string
		if err = json.Unmarshal(p.headers, &h); err != nil {
			return err
		}
		// Продолжаем исходный checkout trace, а не trace периодического polling.
		sendCtx := telemetry.FromHeaders(ctx, h)
		sendCtx, c := context.WithTimeout(sendCtx, 5*time.Second)
		err = producer.Publish(sendCtx, p.topic, p.key, p.id, p.payload)
		if err == nil {
			dbCtx, done := r.t.DB(sendCtx, "UPDATE", "outbox")
			_, err = r.db.ExecContext(dbCtx, `UPDATE outbox SET published_at=now() WHERE event_id=$1`, p.id)
			done(err)
		}
		c()
		if err != nil {
			return err
		}
		// Crash между publish и UPDATE приводит к повтору. Event ID остаётся тем же;
		// consumer дедуплицирует его вместе со своей business transaction.
	}
	return nil
}
