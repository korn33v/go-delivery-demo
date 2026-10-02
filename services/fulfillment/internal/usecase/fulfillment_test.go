package usecase

import (
	"context"
	"errors"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/fulfillment/internal/domain"
	"testing"
)

type fake struct {
	accepts    int
	f          domain.Fulfillment
	advanceErr error
}

func (f *fake) Accept(_ context.Context, e string, x domain.Fulfillment, out string) (bool, error) {
	f.accepts++
	f.f = x
	return true, nil
}
func (f *fake) Get(context.Context, string) (domain.Fulfillment, error) { return f.f, nil }
func (f *fake) Advance(context.Context, string, string) (domain.Fulfillment, error) {
	return f.f, f.advanceErr
}
func TestAcceptRequiresPayment(t *testing.T) {
	f := &fake{}
	s := New(f, telemetry.NewNoop("fulfillment-test"))
	if err := s.Accept(context.Background(), "event-1", "order-1", "", 700); !errors.Is(err, domain.ErrInvalid) || f.accepts != 0 {
		t.Fatal("unpaid accepted")
	}
	if err := s.Accept(context.Background(), "event-1", "order-1", "p", 700); err != nil || f.f.Status != "Accepted" || f.f.Sequence != 1 {
		t.Fatal("valid paid event rejected")
	}
}
func TestAdvancePropagatesDomainError(t *testing.T) {
	f := &fake{advanceErr: domain.ErrCompleted}
	s := New(f, telemetry.NewNoop("fulfillment-test"))
	_, err := s.Advance(context.Background(), "o")
	if !errors.Is(err, domain.ErrCompleted) {
		t.Fatal(err)
	}
}
