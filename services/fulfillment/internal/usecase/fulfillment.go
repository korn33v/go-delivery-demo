package usecase

import (
	"context"
	"godelivery/internal/platform/app"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/fulfillment/internal/domain"
	"time"
)

type Store interface {
	Accept(context.Context, string, domain.Fulfillment, string) (bool, error)
	Get(context.Context, string) (domain.Fulfillment, error)
	Advance(context.Context, string, string) (domain.Fulfillment, error)
}
type Service struct {
	store Store
	t     *telemetry.Telemetry
}

func New(store Store, t *telemetry.Telemetry) *Service { return &Service{store, t} }
func (s *Service) Accept(ctx context.Context, eventID, orderID, paymentID string, total int64) error {
	ctx, span := s.t.Tracer.Start(ctx, "Fulfillment accept paid order")
	defer span.End()
	if orderID == "" || paymentID == "" || total <= 0 {
		return domain.ErrInvalid
	}
	now := time.Now().UTC()
	// В demo fulfillment_id = order_id: преподаватель использует одну переменную ORDER_ID.
	f := domain.Fulfillment{ID: orderID, OrderID: orderID, Status: "Accepted", Sequence: 1, CreatedAt: now, UpdatedAt: now, TraceHeaders: telemetry.TraceHeaders(ctx)}
	created, err := s.store.Accept(ctx, eventID, f, app.ID("event"))
	if err != nil {
		telemetry.Fail(span, err)
		return err
	}
	if created {
		s.t.Count(ctx, "fulfillments_started", 1)
		s.t.Logger.InfoContext(ctx, "fulfillment accepted", "operation", "fulfillment.accept", "order_id", orderID, "payment_id", paymentID)
	}
	return nil
}
func (s *Service) Get(ctx context.Context, id string) (domain.Fulfillment, error) {
	return s.store.Get(ctx, id)
}
func (s *Service) Advance(ctx context.Context, id string) (domain.Fulfillment, error) {
	ctx, span := s.t.Tracer.Start(ctx, "Fulfillment advance")
	defer span.End()
	f, err := s.store.Advance(ctx, id, app.ID("event"))
	if err != nil {
		telemetry.Fail(span, err)
		return f, err
	}
	s.t.Logger.InfoContext(ctx, "fulfillment advanced", "operation", "fulfillment.advance", "order_id", f.OrderID, "status", f.Status)
	if f.Status == "Completed" {
		s.t.Count(ctx, "fulfillments_completed", 1)
	}
	return f, nil
}
