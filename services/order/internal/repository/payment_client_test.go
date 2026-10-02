package repository

import (
	"context"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"godelivery/internal/platform/telemetry"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPaymentClientPropagatesTrace(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	ctx, span := provider.Tracer("test").Start(context.Background(), "checkout")
	defer span.End()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("traceparent") == "" {
			t.Error("missing traceparent")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":"payment-1","status":"Succeeded"}`))
	}))
	defer server.Close()
	c := NewPaymentClient(server.URL, time.Second, telemetry.NewNoop("order-test"))
	p, err := c.Pay(ctx, "order-1", 700, "tok-success")
	if err != nil || p.ID != "payment-1" {
		t.Fatalf("%+v %v", p, err)
	}
}
func TestPaymentClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	c := NewPaymentClient(server.URL, 10*time.Millisecond, telemetry.NewNoop("order-test"))
	_, err := c.Pay(context.Background(), "o", 700, "tok-success")
	if err == nil {
		t.Fatal("expected timeout")
	}
}
func TestPaymentClientMapsDecline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(402)
		_, _ = w.Write([]byte(`{"payment":{"id":"p","status":"Declined"}}`))
	}))
	defer server.Close()
	c := NewPaymentClient(server.URL, time.Second, telemetry.NewNoop("order-test"))
	p, err := c.Pay(context.Background(), "o", 700, "tok-decline")
	if err != nil || p.Status != "Declined" {
		t.Fatalf("%+v %v", p, err)
	}
}

// Проверяем входящую W3C extraction без глобального изменения tracer provider.
func TestTraceHeadersRoundTrip(t *testing.T) {
	id, _ := trace.TraceIDFromHex("11111111111111111111111111111111")
	sid, _ := trace.SpanIDFromHex("2222222222222222")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: id, SpanID: sid, TraceFlags: trace.FlagsSampled}))
	headers := telemetry.TraceHeaders(ctx)
	got := trace.SpanContextFromContext(telemetry.FromHeaders(context.Background(), headers))
	if got.TraceID() != id || !got.IsRemote() {
		t.Fatal("lost context")
	}
	_ = otel.GetTracerProvider()
}
