package delivery

import (
	"context"
	"encoding/json"
	"github.com/segmentio/kafka-go"
	"godelivery/contracts/events"
	"godelivery/internal/platform/bus"
	"godelivery/services/fulfillment/internal/usecase"
)

func PaidConsumer(s *usecase.Service) func(context.Context, kafka.Message) error {
	return func(ctx context.Context, m kafka.Message) error {
		var e events.OrderPaid
		if err := json.Unmarshal(m.Value, &e); err != nil {
			return err
		}
		if err := bus.Validate(string(m.Key), e.EventID, e.EventType, events.PaidTopic, e.OrderID, e.EventVersion); err != nil {
			return err
		}
		return s.Accept(ctx, e.EventID, e.OrderID, e.PaymentID, e.Total)
	}
}
