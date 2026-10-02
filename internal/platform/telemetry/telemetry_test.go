package telemetry

import (
	"compress/gzip"
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	logpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Проверяем настоящий OTLP/HTTP wire format, flush и safe attributes.
// Test receiver не заменяет runtime-проверку deployed Collector/Loki/Jaeger.
func TestOTLPExportsLogsMetricsAndDBSpans(t *testing.T) {
	var mu sync.Mutex
	received := map[string][][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			defer gz.Close()
			reader = gz
		}
		b, err := io.ReadAll(reader)
		if err != nil {
			t.Error(err)
		}
		mu.Lock()
		received[r.URL.Path] = append(received[r.URL.Path], b)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(200)
	}))
	defer server.Close()
	previousTrace, previousMeter := otel.GetTracerProvider(), otel.GetMeterProvider()
	defer otel.SetTracerProvider(previousTrace)
	defer otel.SetMeterProvider(previousMeter)
	tel, err := New(context.Background(), "order-service", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := tel.Tracer.Start(context.Background(), "checkout-test")
	_, finish := tel.DB(ctx, "INSERT", "orders")
	finish(nil)
	tel.Count(ctx, "orders_created", 1)
	tel.Observe(ctx, "http_server_request_duration_seconds", .01, attribute.String("route", "/api/v1/orders/{id}"))
	// Между checkout и ручным Completed на семинаре может пройти целый час.
	tel.Observe(ctx, "order_processing_duration_seconds", 3600)
	tel.Logger.InfoContext(ctx, "order paid", "operation", "order.checkout", "order_id", "order-test")
	span.End()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = tel.Shutdown(shutdown); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, path := range []string{"/v1/traces", "/v1/metrics", "/v1/logs"} {
		if len(received[path]) == 0 {
			keys := []string{}
			for k := range received {
				keys = append(keys, k)
			}
			t.Fatalf("OTLP %s missing, paths=%v", path, keys)
		}
	}
	foundDB := false
	for _, b := range received["/v1/traces"] {
		var req tracepb.ExportTraceServiceRequest
		if err = proto.Unmarshal(b, &req); err != nil {
			t.Fatal(err)
		}
		for _, rs := range req.ResourceSpans {
			for _, scope := range rs.ScopeSpans {
				for _, s := range scope.Spans {
					if s.Name == "SQL INSERT orders" {
						foundDB = true
						if s.EndTimeUnixNano < s.StartTimeUnixNano || len(s.TraceId) != 16 {
							t.Fatal("invalid DB span timing/trace")
						}
						attrs := map[string]string{}
						for _, a := range s.Attributes {
							attrs[a.Key] = a.Value.GetStringValue()
						}
						if attrs["db.system"] != "postgresql" || attrs["db.operation"] != "INSERT" || attrs["db.collection"] != "orders" || attrs["db.namespace"] != "order_db" {
							t.Fatalf("DB attributes %v", attrs)
						}
					}
				}
			}
		}
	}
	if !foundDB {
		t.Fatal("DB span absent")
	}
	foundCounter := false
	foundBusinessDuration := false
	for _, b := range received["/v1/metrics"] {
		var req metricpb.ExportMetricsServiceRequest
		if err = proto.Unmarshal(b, &req); err != nil {
			t.Fatal(err)
		}
		for _, rs := range req.ResourceMetrics {
			for _, scope := range rs.ScopeMetrics {
				for _, m := range scope.Metrics {
					if m.Name == "order_processing_duration_seconds" {
						for _, point := range m.GetHistogram().DataPoints {
							for _, bound := range point.ExplicitBounds {
								foundBusinessDuration = foundBusinessDuration || bound >= 3600
							}
						}
					}
					if m.Name == "orders_created" {
						foundCounter = true
						for _, point := range m.GetSum().DataPoints {
							for _, a := range point.Attributes {
								if a.Key == "order_id" || a.Key == "payment_id" || a.Key == "trace_id" || a.Key == "event_id" {
									t.Fatal("high cardinality label", a.Key)
								}
							}
						}
					}
				}
			}
		}
	}
	if !foundCounter {
		t.Fatal("business counter absent")
	}
	if !foundBusinessDuration {
		t.Fatal("business histogram cannot represent a one-hour seminar workflow")
	}
	foundLog := false
	for _, b := range received["/v1/logs"] {
		var req logpb.ExportLogsServiceRequest
		if err = proto.Unmarshal(b, &req); err != nil {
			t.Fatal(err)
		}
		for _, rs := range req.ResourceLogs {
			for _, scope := range rs.ScopeLogs {
				for _, record := range scope.LogRecords {
					body := record.Body.GetStringValue()
					if strings.Contains(body, `"order_id":"order-test"`) && strings.Contains(body, `"trace_id":`) && len(record.TraceId) == 16 {
						foundLog = true
					}
				}
			}
		}
	}
	if !foundLog {
		t.Fatal("structured log correlation missing")
	}
}
