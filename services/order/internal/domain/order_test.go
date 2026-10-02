package domain

import (
	"errors"
	"testing"
)

func TestTotal(t *testing.T) {
	for _, tc := range []struct {
		name, customer, restaurant string
		items                      []Item
		total                      int64
		bad                        bool
	}{
		{"menu prices", "customer-1", "restaurant-1", []Item{{"pizza-margherita", 2}, {"pizza-pepperoni", 1}}, 2300, false},
		{"unknown dish", "customer-1", "restaurant-1", []Item{{"missing", 1}}, 0, true},
		{"zero quantity", "customer-1", "restaurant-1", []Item{{"pizza-margherita", 0}}, 0, true},
		{"too many", "customer-1", "restaurant-1", []Item{{"pizza-margherita", 21}}, 0, true},
		{"other restaurant", "customer-1", "restaurant-2", []Item{{"pizza-margherita", 1}}, 0, true},
		{"empty", "customer-1", "restaurant-1", nil, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			total, err := Total(tc.customer, tc.restaurant, tc.items)
			if tc.bad {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("want invalid, got %v", err)
				}
			} else if err != nil || total != tc.total {
				t.Fatalf("total=%d err=%v", total, err)
			}
		})
	}
}
func TestOrderEventOrdering(t *testing.T) {
	o := Order{Status: Paid}
	if err := o.CanApply("Accepted", 1); err != nil {
		t.Fatal(err)
	}
	if err := o.CanApply("Ready", 3); !errors.Is(err, ErrTransition) {
		t.Fatal("gap accepted")
	}
	o.Status = "Cooking"
	o.Sequence = 2
	if err := o.CanApply("Accepted", 1); err != nil {
		t.Fatal("old sequence should not roll back")
	}
	o.Status = PaymentDeclined
	o.Sequence = 0
	if err := o.CanApply("Accepted", 1); !errors.Is(err, ErrTransition) {
		t.Fatal("declined order progressed")
	}
}
