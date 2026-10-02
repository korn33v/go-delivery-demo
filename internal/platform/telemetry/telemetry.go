package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Telemetry struct {
	Service    string
	Logger     *slog.Logger
	Tracer     trace.Tracer
	counters   map[string]metric.Int64Counter
	histograms map[string]metric.Float64Histogram
	status     metric.Int64Gauge
	shutdown   func(context.Context) error
}

// NewNoop используется тестами. Production всегда вызывает New и экспортирует OTLP.
func NewNoop(service string) *Telemetry {
	t, _ := instruments(service, otel.GetMeterProvider().Meter("godelivery"), otel.Tracer("godelivery"))
	t.Logger = slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", service)
	t.shutdown = func(context.Context) error { return nil }
	return t
}

func New(ctx context.Context, service, endpoint string) (*Telemetry, error) {
	res := resource.NewWithAttributes("", attribute.String("service.name", service), attribute.String("service.namespace", "godelivery"))
	te, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint), otlptracehttp.WithURLPath("/v1/traces"))
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithBatcher(te, sdktrace.WithBatchTimeout(time.Second)))
	me, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint), otlpmetrichttp.WithURLPath("/v1/metrics"))
	if err != nil {
		_ = tp.Shutdown(ctx)
		return nil, err
	}
	// HTTP/SQL измеряются в миллисекундах, ручной бизнес-процесс семинара — в минутах.
	// Поэтому общий HTTP bucket 10s нельзя использовать как верхнюю границу доставки.
	var views []sdkmetric.View
	for _, name := range []string{"http_server_request_duration_seconds", "db_operation_duration_seconds", "service_call_duration_seconds"} {
		views = append(views, sdkmetric.NewView(sdkmetric.Instrument{Name: name, Kind: sdkmetric.InstrumentKindHistogram}, sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}}}))
	}
	views = append(views, sdkmetric.NewView(sdkmetric.Instrument{Name: "order_processing_duration_seconds", Kind: sdkmetric.InstrumentKindHistogram}, sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: []float64{.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1200, 1800, 3600, 7200, 14400}}}))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithView(views...), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(me, sdkmetric.WithInterval(2*time.Second))))
	le, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(endpoint), otlploghttp.WithURLPath("/v1/logs"))
	if err != nil {
		_ = tp.Shutdown(ctx)
		_ = mp.Shutdown(ctx)
		return nil, err
	}
	lp := sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewBatchProcessor(le, sdklog.WithExportInterval(time.Second))))
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t, err := instruments(service, mp.Meter("godelivery"), tp.Tracer("godelivery"))
	if err != nil {
		return nil, err
	}
	t.Logger = slog.New(&jsonOTLPHandler{service: service, logger: lp.Logger("godelivery"), mu: &sync.Mutex{}})
	t.shutdown = func(ctx context.Context) error {
		a := tp.Shutdown(ctx)
		b := mp.Shutdown(ctx)
		c := lp.Shutdown(ctx)
		if a != nil {
			return a
		}
		if b != nil {
			return b
		}
		return c
	}
	return t, nil
}

func instruments(service string, meter metric.Meter, tracer trace.Tracer) (*Telemetry, error) {
	t := &Telemetry{Service: service, Tracer: tracer, counters: map[string]metric.Int64Counter{}, histograms: map[string]metric.Float64Histogram{}}
	for _, name := range []string{"http_server_requests", "handler_errors", "kafka_messages_produced", "kafka_messages_consumed", "kafka_consumer_errors", "service_calls", "orders_created", "orders_paid", "orders_completed", "orders_failed", "payments_success", "payments_failed", "fulfillments_started", "fulfillments_completed"} {
		c, err := meter.Int64Counter(name)
		if err != nil {
			return nil, err
		}
		t.counters[name] = c
	}
	for _, name := range []string{"http_server_request_duration_seconds", "db_operation_duration_seconds", "service_call_duration_seconds", "order_processing_duration_seconds"} {
		h, err := meter.Float64Histogram(name, metric.WithUnit("s"))
		if err != nil {
			return nil, err
		}
		t.histograms[name] = h
	}
	g, err := meter.Int64Gauge("orders_by_status")
	if err != nil {
		return nil, err
	}
	t.status = g
	// Нулевой baseline позволяет Prometheus учитывать первый бизнес-факт в rate/increase.
	// Метрики принадлежат своему сервису; нулевое значение не является бизнес-событием.
	var business []string
	switch service {
	case "order-service":
		business = []string{"orders_created", "orders_paid", "orders_completed", "orders_failed"}
	case "payment-service":
		business = []string{"payments_success", "payments_failed"}
	case "fulfillment-service":
		business = []string{"fulfillments_started", "fulfillments_completed"}
	}
	for _, name := range business {
		t.Count(context.Background(), name, 0)
	}
	return t, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error { return t.shutdown(ctx) }
func (t *Telemetry) Count(ctx context.Context, name string, n int64, attrs ...attribute.KeyValue) {
	t.counters[name].Add(ctx, n, metric.WithAttributes(append(attrs, attribute.String("service", t.Service))...))
}
func (t *Telemetry) Observe(ctx context.Context, name string, seconds float64, attrs ...attribute.KeyValue) {
	t.histograms[name].Record(ctx, seconds, metric.WithAttributes(append(attrs, attribute.String("service", t.Service))...))
}
func (t *Telemetry) Status(ctx context.Context, status string, n int64) {
	t.status.Record(ctx, n, metric.WithAttributes(attribute.String("service", t.Service), attribute.String("status", status)))
}
func (t *Telemetry) Call(ctx context.Context, source, target, protocol string, start time.Time, err error) {
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	a := []attribute.KeyValue{attribute.String("source", source), attribute.String("target", target), attribute.String("protocol", protocol), attribute.String("outcome", outcome)}
	t.Count(ctx, "service_calls", 1, a...)
	t.Observe(ctx, "service_call_duration_seconds", time.Since(start).Seconds(), a...)
}

func (t *Telemetry) InitCalls(source, target, protocol string) {
	// Карта фильтрует increase > 0: этот baseline не рисует ребро до реального вызова.
	for _, outcome := range []string{"success", "error"} {
		t.Count(context.Background(), "service_calls", 0, attribute.String("source", source), attribute.String("target", target), attribute.String("protocol", protocol), attribute.String("outcome", outcome))
	}
}

// DB создаёт span вокруг реального SQL, включая scan/commit. Statement values не записываем.
func (t *Telemetry) DB(ctx context.Context, operation, table string) (context.Context, func(error)) {
	start := time.Now()
	ctx, span := t.Tracer.Start(ctx, "SQL "+operation+" "+table, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("db.system", "postgresql"), attribute.String("db.operation", operation), attribute.String("db.collection", table), attribute.String("db.namespace", strings.TrimSuffix(t.Service, "-service")+"_db")))
	return ctx, func(err error) {
		Fail(span, err)
		t.Observe(ctx, "db_operation_duration_seconds", time.Since(start).Seconds(), attribute.String("operation", operation), attribute.String("table", table))
		span.End()
	}
}
func Fail(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// Context содержит только lifetime и W3C trace context, никогда транзакцию/repository.
func TraceHeaders(ctx context.Context) map[string]string {
	c := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, c)
	return map[string]string(c)
}
func FromHeaders(ctx context.Context, h map[string]string) context.Context {
	return propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier(h))
}

// Один slog.Handler пишет одинаковое JSON-тело в stdout и OTLP Logs.
// Loki хранит trace_id/order_id как metadata, а не как индексные labels.
type jsonOTLPHandler struct {
	service string
	logger  otellog.Logger
	attrs   []slog.Attr
	group   string
	mu      *sync.Mutex
}

func (h *jsonOTLPHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *jsonOTLPHandler) WithAttrs(a []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(append([]slog.Attr{}, h.attrs...), a...)
	return &c
}
func (h *jsonOTLPHandler) WithGroup(g string) slog.Handler { c := *h; c.group = g; return &c }
func (h *jsonOTLPHandler) Handle(ctx context.Context, r slog.Record) error {
	fields := map[string]any{"timestamp": r.Time.UTC().Format(time.RFC3339Nano), "level": r.Level.String(), "service": h.service, "message": r.Message}
	set := func(a slog.Attr) {
		key := a.Key
		if h.group != "" {
			key = h.group + "." + key
		}
		fields[key] = a.Value.Resolve().Any()
	}
	for _, a := range h.attrs {
		set(a)
	}
	r.Attrs(func(a slog.Attr) bool { set(a); return true })
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		fields["trace_id"] = sc.TraceID().String()
		fields["span_id"] = sc.SpanID().String()
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	h.mu.Lock()
	_, err = fmt.Fprintln(os.Stdout, string(body))
	h.mu.Unlock()
	var record otellog.Record
	record.SetTimestamp(r.Time)
	record.SetObservedTimestamp(time.Now())
	record.SetBody(attribute.StringValue(string(body)))
	record.SetSeverity(otellog.SeverityInfo)
	if r.Level >= slog.LevelError {
		record.SetSeverity(otellog.SeverityError)
	}
	record.SetSeverityText(r.Level.String())
	for k, v := range fields {
		record.AddAttributes(attribute.String(k, fmt.Sprint(v)))
	}
	h.logger.Emit(ctx, record)
	return err
}
