package usecase

import (
	"context"
	"godelivery/internal/platform/app"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/payment/internal/domain"
	"time"
)

type Store interface {
	Save(context.Context, domain.Payment) (domain.Payment, bool, error)
	Get(context.Context, string) (domain.Payment, error)
}
type Service struct {
	store Store
	t     *telemetry.Telemetry
}

func New(store Store, t *telemetry.Telemetry) *Service { return &Service{store, t} }
func (s *Service) Pay(ctx context.Context, orderID string, total int64, token string) (domain.Payment, error) {
	ctx, span := s.t.Tracer.Start(ctx, "Payment authorize")
	defer span.End()
	status, err := domain.Decide(orderID, total, token)
	if err != nil {
		return domain.Payment{}, err
	}
	if token == "tok-slow" {
		select {
		case <-ctx.Done():
			telemetry.Fail(span, ctx.Err())
			return domain.Payment{}, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	p, created, err := s.store.Save(ctx, domain.Payment{ID: app.ID("payment"), OrderID: orderID, Status: status, Total: total, CreatedAt: time.Now().UTC()})
	if err != nil {
		telemetry.Fail(span, err)
		return p, err
	}
	if created {
		if p.Status == "Succeeded" {
			s.t.Count(ctx, "payments_success", 1)
			s.t.Logger.InfoContext(ctx, "payment succeeded", "operation", "payment.authorize", "order_id", orderID, "payment_id", p.ID)
		} else {
			s.t.Count(ctx, "payments_failed", 1)
			s.t.Logger.ErrorContext(ctx, "payment declined", "operation", "payment.authorize", "order_id", orderID, "payment_id", p.ID, "error", "demo_decline")
		}
	}
	return p, nil
}
func (s *Service) Get(ctx context.Context, id string) (domain.Payment, error) {
	return s.store.Get(ctx, id)
}
