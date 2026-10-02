package delivery

import (
	"context"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/order/internal/domain"
	"godelivery/services/order/internal/usecase"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type storeFake struct {
	o      domain.Order
	events int
}

func (f *storeFake) Create(_ context.Context, o domain.Order) error { f.o = o; return nil }
func (f *storeFake) Get(_ context.Context, id string) (domain.Order, error) {
	if id != f.o.ID {
		return domain.Order{}, domain.ErrNotFound
	}
	return f.o, nil
}
func (f *storeFake) MarkPaid(_ context.Context, id, p, e string) error {
	f.o.Status = domain.Paid
	f.events++
	return nil
}
func (f *storeFake) MarkFailed(_ context.Context, id, status, p string) error {
	f.o.Status = status
	return nil
}
func (f *storeFake) ApplyStatus(context.Context, string, string, int, string) (bool, domain.Order, error) {
	return false, f.o, nil
}

type paymentStub struct{}

func (paymentStub) Pay(_ context.Context, id string, total int64, token string) (usecase.Payment, error) {
	status := "Succeeded"
	if token == "tok-decline" {
		status = "Declined"
	}
	return usecase.Payment{ID: "p", Status: status}, nil
}
func TestHTTPCheckout(t *testing.T) {
	f := &storeFake{}
	s := usecase.New(f, paymentStub{}, telemetry.NewNoop("order-test"))
	mux := http.NewServeMux()
	New(s).Routes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	for _, tc := range []struct {
		method, path, body string
		code               int
	}{
		{"GET", "/api/v1/menu", "", 200},
		{"POST", "/api/v1/orders", `{"customer_id":"customer-1","restaurant_id":"restaurant-1","items":[{"dish_id":"pizza-margherita","quantity":1}],"payment_token":"tok-success"}`, 201},
		{"POST", "/api/v1/orders", `{"customer_id":"customer-1","restaurant_id":"restaurant-1","items":[{"dish_id":"pizza-margherita","quantity":1}],"payment_token":"tok-decline"}`, 402},
		{"POST", "/api/v1/orders", `{"customer_id":"unknown"}`, 422},
		{"POST", "/api/v1/orders", `{"unknown":1}`, 400},
		{"GET", "/api/v1/orders/missing", "", 404},
	} {
		req, _ := http.NewRequest(tc.method, server.URL+tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.code {
			t.Fatalf("%s: got %d want %d", tc.path, res.StatusCode, tc.code)
		}
	}
	if f.events != 1 {
		t.Fatal("decline produced an event")
	}
}
