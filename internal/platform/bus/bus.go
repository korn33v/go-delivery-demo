// Package bus — транспортная механика Kafka. Бизнес-обработчики принадлежат сервисам.
package bus

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"godelivery/internal/platform/telemetry"
)

type Producer struct {
	writer *kafka.Writer
	t      *telemetry.Telemetry
}

func NewProducer(broker string, t *telemetry.Telemetry) *Producer {
	t.InitCalls(t.Service, "kafka", "kafka")
	return &Producer{writer: &kafka.Writer{Addr: kafka.TCP(broker), Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll, Async: false, WriteTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, BatchTimeout: 10 * time.Millisecond}, t: t}
}
func (p *Producer) Close() error { return p.writer.Close() }
func (p *Producer) Publish(ctx context.Context, topic, key, eventID string, payload []byte) error {
	start := time.Now()
	ctx, span := p.t.Tracer.Start(ctx, "Kafka publish "+topic, trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(attribute.String("messaging.system", "kafka"), attribute.String("messaging.destination.name", topic), attribute.String("messaging.operation.type", "send"), attribute.String("messaging.message.id", eventID)))
	defer span.End()
	headers := []kafka.Header{{Key: "event_id", Value: []byte(eventID)}}
	for k, v := range telemetry.TraceHeaders(ctx) {
		headers = append(headers, kafka.Header{Key: k, Value: []byte(v)})
	}
	err := p.writer.WriteMessages(ctx, kafka.Message{Topic: topic, Key: []byte(key), Value: payload, Headers: headers})
	telemetry.Fail(span, err)
	p.t.Call(ctx, p.t.Service, "kafka", "kafka", start, err)
	if err == nil {
		p.t.Count(ctx, "kafka_messages_produced", 1, attribute.String("topic", topic))
		p.t.Logger.InfoContext(ctx, "event published", "operation", "kafka.publish", "topic", topic, "event_id", eventID, "order_id", key)
	}
	return err
}

type Consumer struct {
	reader       *kafka.Reader
	t            *telemetry.Telemetry
	topic, group string
}

func NewConsumer(broker, topic, group string, t *telemetry.Telemetry) *Consumer {
	t.InitCalls("kafka", t.Service, "kafka")
	t.Count(context.Background(), "kafka_consumer_errors", 0, attribute.String("topic", topic), attribute.String("group", group))
	return &Consumer{reader: kafka.NewReader(kafka.ReaderConfig{Brokers: []string{broker}, Topic: topic, GroupID: group, StartOffset: kafka.FirstOffset, MinBytes: 1, MaxBytes: 1e6, MaxWait: 500 * time.Millisecond, CommitInterval: 0}), t: t, topic: topic, group: group}
}
func (c *Consumer) Close() error { return c.reader.Close() }

// Fetch -> business transaction -> CommitMessages. При ошибке повторяем ЭТО сообщение,
// не читаем следующее, иначе commit более позднего offset потеряет failed message.
func (c *Consumer) Run(ctx context.Context, handler func(context.Context, kafka.Message) error) error {
	for ctx.Err() == nil {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.t.Logger.ErrorContext(ctx, "Kafka fetch failed", "operation", "kafka.fetch", "error", err.Error())
			c.t.Count(ctx, "kafka_consumer_errors", 1, attribute.String("topic", c.topic), attribute.String("group", c.group))
			if !pause(ctx) {
				return nil
			}
			continue
		}
		for ctx.Err() == nil {
			start := time.Now()
			h := map[string]string{}
			for _, a := range msg.Headers {
				h[a.Key] = string(a.Value)
			}
			messageCtx := telemetry.FromHeaders(ctx, h)
			messageCtx, span := c.t.Tracer.Start(messageCtx, "Kafka consume "+c.topic, trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(attribute.String("messaging.system", "kafka"), attribute.String("messaging.destination.name", c.topic), attribute.String("messaging.consumer.group.name", c.group), attribute.Int("messaging.kafka.partition", msg.Partition), attribute.Int64("messaging.kafka.offset", msg.Offset)))
			messageCtx, cancel := context.WithTimeout(messageCtx, 5*time.Second)
			err = handler(messageCtx, msg)
			if err == nil {
				err = c.reader.CommitMessages(messageCtx, msg)
			}
			telemetry.Fail(span, err)
			c.t.Call(messageCtx, "kafka", c.t.Service, "kafka", start, err)
			if err == nil {
				c.t.Count(messageCtx, "kafka_messages_consumed", 1, attribute.String("topic", c.topic), attribute.String("group", c.group))
				c.t.Logger.InfoContext(messageCtx, "offset committed after DB commit", "operation", "kafka.commit", "topic", c.topic, "group", c.group, "partition", msg.Partition, "offset", msg.Offset, "message_key", string(msg.Key), "order_id", string(msg.Key), "event_id", h["event_id"])
			} else {
				c.t.Count(messageCtx, "kafka_consumer_errors", 1, attribute.String("topic", c.topic), attribute.String("group", c.group))
				c.t.Logger.ErrorContext(messageCtx, "consumer failed; offset not committed", "operation", "kafka.consume", "order_id", string(msg.Key), "offset", msg.Offset, "error", err.Error())
			}
			cancel()
			span.End()
			if err == nil {
				break
			}
			if !pause(ctx) {
				return nil
			}
		}
	}
	return nil
}
func pause(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(time.Second):
		return true
	}
}

// Не используем лог/SDK-сущности в domain. Проверка envelope происходит на transport boundary.
func Validate(key, eventID, eventType, wantType, orderID string, version int) error {
	if key != orderID || eventID == "" || orderID == "" || eventType != wantType || version != 1 {
		return fmt.Errorf("invalid %s envelope", wantType)
	}
	return nil
}
