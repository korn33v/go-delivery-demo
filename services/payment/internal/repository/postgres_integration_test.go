//go:build integration

package repository

import (
	"context"
	"errors"
	"godelivery/internal/platform/telemetry"
	"godelivery/internal/platform/testenv"
	"godelivery/services/payment/internal/domain"
	"testing"
	"time"
)

func TestPaymentIdempotency(t *testing.T) {
	db := testenv.Database(t, "PAYMENT_DB_URL", "../../migrations")
	r := NewPostgres(db, telemetry.NewNoop("payment-test"))
	ctx := context.Background()
	p := domain.Payment{ID: "p1", OrderID: "o1", Total: 700, Status: "Succeeded", CreatedAt: time.Now().UTC()}
	saved, created, err := r.Save(ctx, p)
	if err != nil || !created || saved.ID != "p1" {
		t.Fatal(err)
	}
	p.ID = "p2"
	saved, created, err = r.Save(ctx, p)
	if err != nil || created || saved.ID != "p1" {
		t.Fatal("retry created a new payment")
	}
	p.Total = 900
	if _, _, err = r.Save(ctx, p); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("amount conflict ignored")
	}
	got, err := r.Get(ctx, "p1")
	if err != nil || got.Total != 700 {
		t.Fatal(err)
	}
}
