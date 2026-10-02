package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/order/internal/usecase"
	"net/http"
	"time"
)

type PaymentClient struct {
	base string
	http *http.Client
	t    *telemetry.Telemetry
}

func NewPaymentClient(base string, timeout time.Duration, t *telemetry.Telemetry) *PaymentClient {
	t.InitCalls(t.Service, "payment-service", "http")
	return &PaymentClient{base, &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, t}
}
func (c *PaymentClient) Pay(ctx context.Context, orderID string, total int64, token string) (payment usecase.Payment, err error) {
	start := time.Now()
	ctx, span := c.t.Tracer.Start(ctx, "POST payment-service /api/v1/payments", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("server.address", "payment-service"), attribute.String("http.request.method", "POST")))
	defer func() {
		telemetry.Fail(span, err)
		callErr := err
		if payment.Status == "Declined" {
			callErr = fmt.Errorf("payment declined")
		}
		c.t.Call(ctx, c.t.Service, "payment-service", "http", start, callErr)
		span.End()
	}()
	// Transport DTO намеренно не совпадает с domain.Order. Token не пишется в БД/telemetry.
	input := struct {
		OrderID string `json:"order_id"`
		Total   int64  `json:"total"`
		Token   string `json:"token"`
	}{orderID, total, token}
	data, err := json.Marshal(input)
	if err != nil {
		return payment, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.base+"/api/v1/payments", bytes.NewReader(data))
	if err != nil {
		return payment, err
	}
	req.Header.Set("Content-Type", "application/json")
	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(req.Header))
	res, err := c.http.Do(req)
	if err != nil {
		return payment, err
	}
	defer res.Body.Close()
	span.SetAttributes(attribute.Int("http.response.status_code", res.StatusCode))
	if res.StatusCode == 201 || res.StatusCode == 200 {
		var dto struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err = json.NewDecoder(res.Body).Decode(&dto); err != nil {
			return payment, err
		}
		if dto.ID == "" || dto.Status != "Succeeded" {
			return payment, fmt.Errorf("invalid payment response")
		}
		return usecase.Payment{ID: dto.ID, Status: dto.Status}, nil
	}
	if res.StatusCode == 402 {
		var dto struct {
			Payment struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"payment"`
		}
		if err = json.NewDecoder(res.Body).Decode(&dto); err != nil {
			return payment, err
		}
		if dto.Payment.ID == "" || dto.Payment.Status != "Declined" {
			return payment, fmt.Errorf("invalid declined response")
		}
		telemetry.Fail(span, fmt.Errorf("payment declined"))
		return usecase.Payment{ID: dto.Payment.ID, Status: "Declined"}, nil
	}
	return payment, fmt.Errorf("payment-service HTTP %d", res.StatusCode)
}
