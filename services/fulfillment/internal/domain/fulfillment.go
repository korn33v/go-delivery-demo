package domain

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("fulfillment not found")
var ErrCompleted = errors.New("fulfillment is already completed")
var ErrInvalid = errors.New("invalid fulfillment event")

type Fulfillment struct {
	ID, OrderID, Status  string
	Sequence             int
	CreatedAt, UpdatedAt time.Time
	TraceHeaders         map[string]string
}

func Next(status string) (string, error) {
	switch status {
	case "Accepted":
		return "Cooking", nil
	case "Cooking":
		return "Ready", nil
	case "Ready":
		return "Delivering", nil
	case "Delivering":
		return "Completed", nil
	case "Completed":
		return "", ErrCompleted
	}
	return "", ErrInvalid
}
