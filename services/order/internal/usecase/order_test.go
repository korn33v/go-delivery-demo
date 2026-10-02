package usecase

import (
	"context"
	"errors"
	"godelivery/internal/platform/telemetry"
	"godelivery/services/order/internal/domain"
	"testing"
)

// Fake хранит состояние; Stub предоставляет заранее выбранный результат Payment.
type fakeStore struct {
	o      domain.Order
	paid   int
	events int
	failed int
}

func (f *fakeStore) Create(_ context.Context, o domain.Order) error    { f.o = o; return nil }
func (f *fakeStore) Get(context.Context, string) (domain.Order, error) { return f.o, nil }
func (f *fakeStore) MarkPaid(_ context.Context, id, payment, event string) error {
	f.paid++
	f.events++
	f.o.Status = domain.Paid
	f.o.PaymentID = payment
	return nil
}
func (f *fakeStore) MarkFailed(ctx context.Context, id, status, payment string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.failed++
	f.o.Status = status
	return nil
}
func (f *fakeStore) ApplyStatus(context.Context, string, string, int, string) (bool, domain.Order, error) {
	return false, f.o, nil
}

type paymentStub struct {
	result Payment
	err    error
	calls  int
	total  int64
	cancel context.CancelFunc
}

func (p *paymentStub) Pay(_ context.Context, id string, total int64, token string) (Payment, error) {
	p.calls++
	p.total = total
	if p.cancel != nil {
		p.cancel()
	}
	return p.result, p.err
}
func valid() CreateInput {
	return CreateInput{CustomerID: "customer-1", RestaurantID: "restaurant-1", Items: []domain.Item{{DishID: "pizza-margherita", Quantity: 2}}, Token: "tok-success"}
}
func TestCheckoutSuccess(t *testing.T) {
	// Arrange
	f := &fakeStore{}
	p := &paymentStub{result: Payment{ID: "payment-1", Status: "Succeeded"}}
	s := New(f, p, telemetry.NewNoop("order-test"))
	// Act
	o, err := s.Create(context.Background(), valid())
	// Assert: event создаётся только после успешного REST-результата.
	if err != nil || o.Status != domain.Paid || p.total != 1400 || p.calls != 1 || f.events != 1 {
		t.Fatalf("order=%+v store=%+v payment=%+v err=%v", o, f, p, err)
	}
}
func TestDeclineCannotStartFulfillment(t *testing.T) {
	f := &fakeStore{}
	p := &paymentStub{result: Payment{ID: "p", Status: "Declined"}}
	s := New(f, p, telemetry.NewNoop("order-test"))
	o, err := s.Create(context.Background(), valid())
	if !errors.Is(err, domain.ErrDeclined) || o.Status != domain.PaymentDeclined || f.events != 0 || f.paid != 0 || f.failed != 1 {
		t.Fatalf("order=%+v store=%+v err=%v", o, f, err)
	}
}
func TestInvalidOrderNeverCallsPayment(t *testing.T) {
	f := &fakeStore{}
	p := &paymentStub{}
	s := New(f, p, telemetry.NewNoop("order-test"))
	in := valid()
	in.Items[0].DishID = "unknown"
	_, err := s.Create(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalid) || p.calls != 0 || f.o.ID != "" {
		t.Fatal("invalid order caused side effects")
	}
}
func TestCancelledCheckoutPersistsUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeStore{}
	p := &paymentStub{err: context.DeadlineExceeded, cancel: cancel}
	s := New(f, p, telemetry.NewNoop("order-test"))
	o, err := s.Create(ctx, valid())
	if !errors.Is(err, domain.ErrPaymentUnavailable) || o.Status != domain.PaymentUnknown || f.failed != 1 || f.events != 0 {
		t.Fatalf("order=%+v err=%v", o, err)
	}
}
