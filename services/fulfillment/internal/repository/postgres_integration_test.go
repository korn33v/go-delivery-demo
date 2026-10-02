//go:build integration

package repository

import (
	"context"
	"errors"
	"godelivery/internal/platform/telemetry"
	"godelivery/internal/platform/testenv"
	"godelivery/services/fulfillment/internal/domain"
	"testing"
	"time"
)

func TestFulfillmentDeduplicatesAndCompletes(t *testing.T) {
	db := testenv.Database(t, "FULFILLMENT_DB_URL", "../../migrations")
	r := NewPostgres(db, telemetry.NewNoop("fulfillment-test"))
	ctx := context.Background()
	now := time.Now().UTC()
	f := domain.Fulfillment{ID: "o1", OrderID: "o1", Status: "Accepted", Sequence: 1, CreatedAt: now, UpdatedAt: now, TraceHeaders: map[string]string{}}
	created, err := r.Accept(ctx, "paid1", f, "accepted1")
	if err != nil || !created {
		t.Fatal(err)
	}
	created, err = r.Accept(ctx, "paid1", f, "accepted2")
	if err != nil || created {
		t.Fatal("duplicate created fulfillment")
	}
	for i, want := range []string{"Cooking", "Ready", "Delivering", "Completed"} {
		got, err := r.Advance(ctx, "o1", string(rune('a'+i)))
		if err != nil || got.Status != want || got.Sequence != i+2 {
			t.Fatalf("%+v %v", got, err)
		}
	}
	if _, err = r.Advance(ctx, "o1", "extra"); !errors.Is(err, domain.ErrCompleted) {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM outbox`).Scan(&n); err != nil || n != 5 {
		t.Fatalf("expected 5 status events, got %d err=%v", n, err)
	}
}
