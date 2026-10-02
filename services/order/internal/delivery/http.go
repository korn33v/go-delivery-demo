package delivery

import (
	"errors"
	"godelivery/internal/platform/telemetry"
	"godelivery/internal/platform/web"
	"godelivery/services/order/internal/domain"
	"godelivery/services/order/internal/usecase"
	"net/http"
	"time"
)

type Handler struct{ service *usecase.Service }

func New(s *usecase.Service) *Handler { return &Handler{s} }

type ItemDTO struct {
	DishID   string `json:"dish_id"`
	Quantity int    `json:"quantity"`
}
type OrderDTO struct {
	ID           string    `json:"id"`
	CustomerID   string    `json:"customer_id"`
	RestaurantID string    `json:"restaurant_id"`
	Items        []ItemDTO `json:"items"`
	Total        int64     `json:"total"`
	Status       string    `json:"status"`
	PaymentID    string    `json:"payment_id"`
	Sequence     int       `json:"sequence"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func orderDTO(o domain.Order) OrderDTO {
	items := []ItemDTO{}
	for _, i := range o.Items {
		items = append(items, ItemDTO{i.DishID, i.Quantity})
	}
	return OrderDTO{o.ID, o.CustomerID, o.RestaurantID, items, o.Total, o.Status, o.PaymentID, o.Sequence, o.CreatedAt, o.UpdatedAt}
}
func (h *Handler) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/menu", h.menu)
	m.HandleFunc("POST /api/v1/orders", h.create)
	m.HandleFunc("GET /api/v1/orders/{id}", h.get)
}
func (h *Handler) menu(w http.ResponseWriter, r *http.Request) {
	type dish struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Price int64  `json:"price"`
	}
	items := []dish{}
	for _, d := range domain.Menu() {
		items = append(items, dish{d.ID, d.Name, d.Price})
	}
	web.JSON(w, 200, map[string]any{"restaurant_id": "restaurant-1", "currency": "RUB", "dishes": items})
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CustomerID   string    `json:"customer_id"`
		RestaurantID string    `json:"restaurant_id"`
		Items        []ItemDTO `json:"items"`
		Token        string    `json:"payment_token"`
	}
	if !web.Decode(w, r, &in) {
		return
	}
	items := []domain.Item{}
	for _, i := range in.Items {
		items = append(items, domain.Item{DishID: i.DishID, Quantity: i.Quantity})
	}
	o, err := h.service.Create(r.Context(), usecase.CreateInput{CustomerID: in.CustomerID, RestaurantID: in.RestaurantID, Items: items, Token: in.Token})
	// Для ручного advance можно продолжить workflow с этим W3C header.
	w.Header().Set("X-Workflow-Traceparent", telemetry.TraceHeaders(r.Context())["traceparent"])
	if err == nil {
		w.Header().Set("Location", "/api/v1/orders/"+o.ID)
		web.JSON(w, 201, orderDTO(o))
		return
	}
	if errors.Is(err, domain.ErrInvalid) {
		web.Error(w, 422, "invalid_order", "Use demo customer/restaurant, existing dishes and a documented token")
		return
	}
	if errors.Is(err, domain.ErrDeclined) || errors.Is(err, domain.ErrPaymentUnavailable) {
		status, code := 402, "payment_declined"
		if errors.Is(err, domain.ErrPaymentUnavailable) {
			status, code = 503, "payment_unknown"
		}
		web.JSON(w, status, struct {
			Code    string   `json:"code"`
			Message string   `json:"message"`
			Order   OrderDTO `json:"order"`
		}{code, err.Error(), orderDTO(o)})
		return
	}
	web.Error(w, 500, "internal_error", "Could not persist checkout")
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	o, err := h.service.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		web.Error(w, 404, "not_found", err.Error())
		return
	}
	if err != nil {
		web.Error(w, 500, "internal_error", "Could not read order")
		return
	}
	web.JSON(w, 200, orderDTO(o))
}
