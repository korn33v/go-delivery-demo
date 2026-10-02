//go:build integration

package repository

import (
	"context"
	"godelivery/internal/platform/telemetry"
	"godelivery/internal/platform/testenv"
	"godelivery/services/order/internal/domain"
	"testing"
	"time"
)

func TestPostgresAtomicPaidAndMonotonicStatus(t *testing.T) {
	db := testenv.Database(t, "ORDER_DB_URL", "../../migrations")
	r := NewPostgres(db, telemetry.NewNoop("order-test"))
	ctx := context.Background()
	now := time.Now().UTC()
	o := domain.Order{ID: "order-test", CustomerID: "customer-1", RestaurantID: "restaurant-1", Items: []domain.Item{{DishID: "pizza-margherita", Quantity: 1}}, Total: 700, Status: domain.Created, CreatedAt: now, UpdatedAt: now}
	if err := r.Create(ctx, o); err != nil {
		t.Fatal(err)
	}
	if err := r.MarkPaid(ctx, o.ID, "payment-test", "paid-test"); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(ctx, o.ID)
	if err != nil || got.Status != domain.Paid || got.PaymentID != "payment-test" {
		t.Fatalf("%+v %v", got, err)
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM outbox WHERE event_id='paid-test'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("outbox count=%d err=%v", n, err)
	}
	changed, _, err := r.ApplyStatus(ctx, "accepted-test", o.ID, 1, "Accepted")
	if err != nil || !changed {
		t.Fatal(err)
	}
	changed, _, err = r.ApplyStatus(ctx, "accepted-test", o.ID, 1, "Accepted")
	if err != nil || changed {
		t.Fatal("duplicate was applied")
	}
	if _, _, err = r.ApplyStatus(ctx, "gap-test", o.ID, 3, "Ready"); err == nil {
		t.Fatal("gap accepted")
	}
	if err = db.QueryRow(`SELECT count(*) FROM processed_events WHERE event_id='gap-test'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("failed business transaction marked event processed")
	}
}
func TestFailedOrderHasNoOutbox(t *testing.T) {
	db := testenv.Database(t, "ORDER_DB_URL", "../../migrations")
	r := NewPostgres(db, telemetry.NewNoop("order-test"))
	ctx := context.Background()
	now := time.Now().UTC()
	o := domain.Order{ID: "failed-test", CustomerID: "customer-1", RestaurantID: "restaurant-1", Items: []domain.Item{{DishID: "pizza-margherita", Quantity: 1}}, Total: 700, Status: domain.Created, CreatedAt: now, UpdatedAt: now}
	if err := r.Create(ctx, o); err != nil {
		t.Fatal(err)
	}
	if err := r.MarkFailed(ctx, o.ID, domain.PaymentDeclined, "p"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM outbox`).Scan(&n); err != nil || n != 0 {
		t.Fatal("failure outbox is not empty")
	}
}
