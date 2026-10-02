package delivery

import (
	"context"
	"encoding/json"
	"github.com/segmentio/kafka-go"
	"godelivery/contracts/events"
	"godelivery/internal/platform/bus"
	"godelivery/services/order/internal/usecase"
)

func StatusConsumer(s *usecase.Service) func(context.Context, kafka.Message) error {
	return func(ctx context.Context, m kafka.Message) error {
		var e events.StatusChanged
		if err := json.Unmarshal(m.Value, &e); err != nil {
			return err
		}
		if err := bus.Validate(string(m.Key), e.EventID, e.EventType, events.StatusTopic, e.OrderID, e.EventVersion); err != nil {
			return err
		}
		return s.ApplyStatus(ctx, e.EventID, e.OrderID, e.Status, e.Sequence)
	}
}
