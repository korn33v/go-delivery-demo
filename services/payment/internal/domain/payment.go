package domain

import (
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid payment")
var ErrNotFound = errors.New("payment not found")
var ErrConflict = errors.New("order already has a payment with another amount")

type Payment struct {
	ID, OrderID, Status string
	Total               int64
	CreatedAt           time.Time
}

func Decide(orderID string, total int64, token string) (string, error) {
	if orderID == "" || len(orderID) > 100 || total <= 0 || total > 1000000 {
		return "", ErrInvalid
	}
	switch token {
	case "tok-success", "tok-slow":
		return "Succeeded", nil
	case "tok-decline":
		return "Declined", nil
	}
	return "", ErrInvalid
}
