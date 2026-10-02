package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"godelivery/internal/platform/telemetry"
)

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, APIError{code, message})
}
func Decode(w http.ResponseWriter, r *http.Request, value any) bool {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		Error(w, 415, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		Error(w, 400, "invalid_json", "Invalid JSON or unknown field")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		Error(w, 400, "invalid_json", "Expected one JSON object")
		return false
	}
	return true
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (w *recorder) WriteHeader(n int) {
	if w.status == 0 {
		w.status = n
		w.ResponseWriter.WriteHeader(n)
	}
}
func (w *recorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}

// Router.Match даёт TEMPLATE пути. ID никогда не становится metric label.
func Instrument(mux *http.ServeMux, t *telemetry.Telemetry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		route := pattern
		if i := strings.IndexByte(route, ' '); i >= 0 {
			route = route[i+1:]
		}
		if route == "" {
			route = "unmatched"
		}
		start := time.Now()
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := t.Tracer.Start(ctx, r.Method+" "+route, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attribute.String("http.request.method", r.Method), attribute.String("http.route", route)))
		defer span.End()
		rw := &recorder{ResponseWriter: w}
		w.Header().Set("X-Trace-ID", span.SpanContext().TraceID().String())
		mux.ServeHTTP(rw, r.WithContext(ctx))
		if rw.status == 0 {
			rw.status = 200
		}
		span.SetAttributes(attribute.Int("http.response.status_code", rw.status))
		attrs := []attribute.KeyValue{attribute.String("route", route), attribute.String("method", r.Method), attribute.String("status", strconv.Itoa(rw.status))}
		t.Count(ctx, "http_server_requests", 1, attrs...)
		t.Observe(ctx, "http_server_request_duration_seconds", time.Since(start).Seconds(), attrs...)
		if rw.status >= 400 {
			t.Count(ctx, "handler_errors", 1, attrs...)
			telemetry.Fail(span, errors.New(http.StatusText(rw.status)))
		}
		if route != "/health" {
			t.Logger.InfoContext(ctx, "HTTP request", "operation", "http.request", "route", route, "method", r.Method, "status", rw.status)
		}
	})
}
func Technical(mux *http.ServeMux, dbHealth func(context.Context) error, apiPath, swaggerPath string) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if dbHealth(ctx) != nil {
			Error(w, 503, "unhealthy", "Database is unavailable")
			return
		}
		JSON(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(apiPath); err != nil {
			Error(w, 500, "contract_missing", "OpenAPI file not found")
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		http.ServeFile(w, r, apiPath)
	})
	mux.Handle("GET /swagger/", http.StripPrefix("/swagger/", http.FileServer(http.Dir(swaggerPath))))
}
