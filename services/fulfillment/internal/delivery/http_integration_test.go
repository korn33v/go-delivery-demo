package delivery

import (
	"context"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/fulfillment/internal/domain"
	"godelivery/services/fulfillment/internal/usecase"
	"net/http"
	"net/http/httptest"
	"testing"
)

type storeFake struct{ f domain.Fulfillment }

func (f *storeFake) Accept(_ context.Context, e string, x domain.Fulfillment, o string) (bool, error) {
	f.f = x
	return true, nil
}
func (f *storeFake) Get(_ context.Context, id string) (domain.Fulfillment, error) {
	if id != f.f.ID {
		return domain.Fulfillment{}, domain.ErrNotFound
	}
	return f.f, nil
}
func (f *storeFake) Advance(_ context.Context, id, e string) (domain.Fulfillment, error) {
	if id != f.f.ID {
		return domain.Fulfillment{}, domain.ErrNotFound
	}
	next, err := domain.Next(f.f.Status)
	if err == nil {
		f.f.Status = next
		f.f.Sequence++
	}
	return f.f, err
}
func TestHTTPLifecycle(t *testing.T) {
	f := &storeFake{f: domain.Fulfillment{ID: "order-1", OrderID: "order-1", Status: "Accepted", Sequence: 1}}
	mux := http.NewServeMux()
	New(usecase.New(f, telemetry.NewNoop("fulfillment-test"))).Routes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	for _, want := range []int{200, 200, 200, 200, 409} {
		res, err := http.Post(server.URL+"/api/v1/fulfillments/order-1/advance", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Fatalf("HTTP %d want %d", res.StatusCode, want)
		}
	}
	res, err := http.Get(server.URL + "/api/v1/fulfillments/missing")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatal(res.StatusCode)
	}
}
