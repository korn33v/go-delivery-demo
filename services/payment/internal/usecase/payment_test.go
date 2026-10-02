package usecase

import (
	"context"
	"errors"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/payment/internal/domain"
	"testing"
	"time"
)

type fake struct {
	calls int
	p     domain.Payment
}

func (f *fake) Save(_ context.Context, p domain.Payment) (domain.Payment, bool, error) {
	f.calls++
	f.p = p
	return p, true, nil
}
func (f *fake) Get(context.Context, string) (domain.Payment, error) { return f.p, nil }
func TestPaymentPersistsDecision(t *testing.T) {
	for _, tc := range []struct{ token, status string }{{"tok-success", "Succeeded"}, {"tok-decline", "Declined"}} {
		f := &fake{}
		s := New(f, telemetry.NewNoop("payment-test"))
		p, err := s.Pay(context.Background(), "order-1", 700, tc.token)
		if err != nil || p.Status != tc.status || f.calls != 1 {
			t.Fatalf("p=%+v err=%v", p, err)
		}
	}
}
func TestSlowPaymentHonorsCancellation(t *testing.T) {
	f := &fake{}
	s := New(f, telemetry.NewNoop("payment-test"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, err := s.Pay(ctx, "order-1", 700, "tok-slow")
	if !errors.Is(err, context.DeadlineExceeded) || f.calls != 0 {
		t.Fatal("cancelled payment persisted")
	}
}
