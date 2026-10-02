// Package events содержит versioned transport DTO, а не domain entities сервисов.
package events

import "time"

const PaidTopic = "order.paid.v1"
const StatusTopic = "fulfillment.status-changed.v1"

type OrderPaid struct {
	EventID      string    `json:"event_id"`
	EventType    string    `json:"event_type"`
	EventVersion int       `json:"event_version"`
	OccurredAt   time.Time `json:"occurred_at"`
	OrderID      string    `json:"order_id"`
	PaymentID    string    `json:"payment_id"`
	Total        int64     `json:"total"`
}
type StatusChanged struct {
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	EventVersion  int       `json:"event_version"`
	OccurredAt    time.Time `json:"occurred_at"`
	OrderID       string    `json:"order_id"`
	FulfillmentID string    `json:"fulfillment_id"`
	Status        string    `json:"status"`
	Sequence      int       `json:"sequence"`
}
