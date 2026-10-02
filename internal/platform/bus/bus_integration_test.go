//go:build integration

package bus

import (
	"context"
	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"godelivery/internal/platform/app"
	"godelivery/internal/platform/telemetry"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestKafkaPublishConsumeAndCommit(t *testing.T) {
	broker := os.Getenv("KAFKA_BROKER")
	if broker == "" {
		t.Fatal("KAFKA_BROKER is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	topic := app.ID("test-publish")
	group := app.ID("test-group")
	conn, err := kafka.DialContext(ctx, "tcp", broker)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := conn.Controller()
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	admin, err := kafka.DialContext(ctx, "tcp", kafka.TCP(controller.Host+":"+strconv.Itoa(controller.Port)).String())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err = admin.CreateTopics(kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}); err != nil {
		t.Fatal(err)
	}
	defer admin.DeleteTopics(topic)
	previous := otel.GetTracerProvider()
	provider := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(previous)
	tel := telemetry.NewNoop("kafka-test")
	producer := NewProducer(broker, tel)
	defer producer.Close()
	consumer := NewConsumer(broker, topic, group, tel)
	defer consumer.Close()
	parent, span := provider.Tracer("test").Start(ctx, "checkout")
	defer span.End()
	if err = producer.Publish(parent, topic, "order-1", "event-1", []byte(`{"event_id":"event-1"}`)); err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		errCh <- consumer.Run(runCtx, func(messageCtx context.Context, m kafka.Message) error {
			if string(m.Key) != "order-1" {
				t.Error("message key lost")
			}
			if trace.SpanContextFromContext(messageCtx).TraceID() != span.SpanContext().TraceID() {
				t.Error("Kafka trace context lost")
			}
			return nil
		})
	}()
	client := &kafka.Client{Addr: kafka.TCP(broker)}
	committed := false
	for ctx.Err() == nil {
		res, e := client.OffsetFetch(ctx, &kafka.OffsetFetchRequest{GroupID: group, Topics: map[string][]int{topic: {0}}})
		if e == nil && res.Error == nil {
			for _, p := range res.Topics[topic] {
				if p.CommittedOffset == 1 {
					committed = true
				}
			}
		}
		if committed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	stop()
	if err = <-errCh; err != nil {
		t.Fatal(err)
	}
	if !committed {
		t.Fatal("consumer did not commit offset after handler")
	}
}
