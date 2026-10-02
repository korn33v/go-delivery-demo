package domain

import (
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid order")
var ErrNotFound = errors.New("order not found")
var ErrDeclined = errors.New("payment declined")
var ErrPaymentUnavailable = errors.New("payment outcome is unknown")
var ErrTransition = errors.New("invalid order transition")

const Created = "Created"
const Paid = "Paid"
const PaymentDeclined = "PaymentDeclined"
const PaymentUnknown = "PaymentUnknown"

var Statuses = []string{Created, Paid, "Accepted", "Cooking", "Ready", "Delivering", "Completed", PaymentDeclined, PaymentUnknown}

type Dish struct {
	ID, Name string
	Price    int64
}

func Menu() []Dish {
	return []Dish{{"pizza-margherita", "Маргарита", 700}, {"pizza-pepperoni", "Пепперони", 900}}
}

type Item struct {
	DishID   string
	Quantity int
}
type Order struct {
	ID, CustomerID, RestaurantID, Status, PaymentID string
	Items                                           []Item
	Total                                           int64
	Sequence                                        int
	CreatedAt, UpdatedAt                            time.Time
}

func Total(customer, restaurant string, items []Item) (int64, error) {
	if customer != "customer-1" || restaurant != "restaurant-1" || len(items) == 0 || len(items) > 10 {
		return 0, ErrInvalid
	}
	var total int64
	for _, item := range items {
		if item.Quantity < 1 || item.Quantity > 20 {
			return 0, ErrInvalid
		}
		found := false
		for _, dish := range Menu() {
			if item.DishID == dish.ID {
				total += dish.Price * int64(item.Quantity)
				found = true
				break
			}
		}
		if !found {
			return 0, ErrInvalid
		}
	}
	return total, nil
}
func StatusForSequence(seq int) string {
	switch seq {
	case 1:
		return "Accepted"
	case 2:
		return "Cooking"
	case 3:
		return "Ready"
	case 4:
		return "Delivering"
	case 5:
		return "Completed"
	}
	return ""
}

// Повтор/старое событие не откатывает состояние. Пропуск sequence считается ошибкой.
func (o Order) CanApply(status string, seq int) error {
	if StatusForSequence(seq) != status {
		return ErrTransition
	}
	if seq <= o.Sequence {
		return nil
	}
	if seq != o.Sequence+1 || o.Status == Created || o.Status == PaymentDeclined || o.Status == PaymentUnknown {
		return ErrTransition
	}
	return nil
}
