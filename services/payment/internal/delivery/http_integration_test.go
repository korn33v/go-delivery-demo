package delivery

import (
	"context"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/payment/internal/domain"
	"godelivery/services/payment/internal/usecase"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type storeFake struct{ p domain.Payment }

func (f *storeFake) Save(_ context.Context, p domain.Payment) (domain.Payment, bool, error) {
	f.p = p
	return p, true, nil
}
func (f *storeFake) Get(_ context.Context, id string) (domain.Payment, error) {
	if id != f.p.ID {
		return domain.Payment{}, domain.ErrNotFound
	}
	return f.p, nil
}
func TestHTTPPayment(t *testing.T) {
	f := &storeFake{}
	mux := http.NewServeMux()
	New(usecase.New(f, telemetry.NewNoop("payment-test"))).Routes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	for _, tc := range []struct {
		token string
		code  int
	}{{"tok-success", 201}, {"tok-decline", 402}, {"invalid", 422}} {
		res, err := http.Post(server.URL+"/api/v1/payments", "application/json", strings.NewReader(`{"order_id":"order-1","total":700,"token":"`+tc.token+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.code {
			t.Fatalf("%s HTTP %d", tc.token, res.StatusCode)
		}
	}
	res, err := http.Get(server.URL + "/api/v1/payments/" + f.p.ID)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
}
