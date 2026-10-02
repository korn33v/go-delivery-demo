package usecase

import (
	"context"
	"godelivery/internal/platform/app"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/order/internal/domain"
	"time"
)

// Порты определяются потребителем: use case зависит от поведения, не SQL/HTTP SDK.
type Store interface {
	Create(context.Context, domain.Order) error
	Get(context.Context, string) (domain.Order, error)
	MarkPaid(context.Context, string, string, string) error
	MarkFailed(context.Context, string, string, string) error
	ApplyStatus(context.Context, string, string, int, string) (bool, domain.Order, error)
}
type Payment struct{ ID, Status string }
type Payments interface {
	Pay(context.Context, string, int64, string) (Payment, error)
}
type Service struct {
	store    Store
	payments Payments
	t        *telemetry.Telemetry
}

func New(store Store, payments Payments, t *telemetry.Telemetry) *Service {
	return &Service{store, payments, t}
}

type CreateInput struct {
	CustomerID, RestaurantID, Token string
	Items                           []domain.Item
}

func (s *Service) Create(ctx context.Context, in CreateInput) (domain.Order, error) {
	ctx, span := s.t.Tracer.Start(ctx, "Order checkout")
	defer span.End()
	total, err := domain.Total(in.CustomerID, in.RestaurantID, in.Items)
	if err != nil || !(in.Token == "tok-success" || in.Token == "tok-decline" || in.Token == "tok-slow") {
		return domain.Order{}, domain.ErrInvalid
	}
	now := time.Now().UTC()
	order := domain.Order{ID: app.ID("order"), CustomerID: in.CustomerID, RestaurantID: in.RestaurantID, Items: in.Items, Total: total, Status: domain.Created, CreatedAt: now, UpdatedAt: now}
	if err = s.store.Create(ctx, order); err != nil {
		telemetry.Fail(span, err)
		return order, err
	}
	s.t.Count(ctx, "orders_created", 1)
	payment, err := s.payments.Pay(ctx, order.ID, total, in.Token)
	if err != nil {
		// Timeout не доказывает, что деньги не списаны. Сохраняем PaymentUnknown.
		// Короткий отдельный cleanup lifetime даёт сохранить диагноз после отмены HTTP.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		order.Status = domain.PaymentUnknown
		saveErr := s.store.MarkFailed(cleanup, order.ID, order.Status, "")
		s.t.Count(cleanup, "orders_failed", 1)
		s.t.Logger.ErrorContext(cleanup, "payment result unknown; fulfillment not started", "operation", "order.checkout", "order_id", order.ID, "error", err.Error())
		telemetry.Fail(span, err)
		if saveErr != nil {
			return order, saveErr
		}
		return order, domain.ErrPaymentUnavailable
	}
	order.PaymentID = payment.ID
	if payment.Status == "Declined" {
		order.Status = domain.PaymentDeclined
		err = s.store.MarkFailed(ctx, order.ID, order.Status, payment.ID)
		s.t.Count(ctx, "orders_failed", 1)
		s.t.Logger.ErrorContext(ctx, "payment declined; no paid event", "operation", "order.checkout", "order_id", order.ID, "payment_id", payment.ID, "error", domain.ErrDeclined.Error())
		telemetry.Fail(span, domain.ErrDeclined)
		if err != nil {
			return order, err
		}
		return order, domain.ErrDeclined
	}
	// Paid + outbox — одна локальная транзакция; Kafka публикуется ПОСЛЕ commit.
	// Это граница между синхронным checkout и асинхронным fulfillment.
	if err = s.store.MarkPaid(ctx, order.ID, payment.ID, app.ID("event")); err != nil {
		telemetry.Fail(span, err)
		return order, err
	}
	order.Status = domain.Paid
	s.t.Count(ctx, "orders_paid", 1)
	s.t.Logger.InfoContext(ctx, "order paid; event queued", "operation", "order.checkout", "order_id", order.ID, "payment_id", payment.ID)
	return order, nil
}
func (s *Service) Get(ctx context.Context, id string) (domain.Order, error) {
	return s.store.Get(ctx, id)
}
func (s *Service) ApplyStatus(ctx context.Context, eventID, orderID, status string, seq int) error {
	ctx, span := s.t.Tracer.Start(ctx, "Order apply fulfillment status")
	defer span.End()
	changed, order, err := s.store.ApplyStatus(ctx, eventID, orderID, seq, status)
	if err != nil {
		telemetry.Fail(span, err)
		return err
	}
	if changed {
		s.t.Logger.InfoContext(ctx, "order status updated", "operation", "order.apply_status", "order_id", orderID, "status", status)
		if status == "Completed" {
			s.t.Count(ctx, "orders_completed", 1)
			s.t.Observe(ctx, "order_processing_duration_seconds", time.Since(order.CreatedAt).Seconds())
		}
	}
	return nil
}
