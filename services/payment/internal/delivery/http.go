package delivery

import (
	"errors"
	"godelivery/internal/platform/web"
	"godelivery/services/payment/internal/domain"
	"godelivery/services/payment/internal/usecase"
	"net/http"
	"time"
)

type Handler struct{ service *usecase.Service }

func New(s *usecase.Service) *Handler { return &Handler{s} }

type PaymentDTO struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"order_id"`
	Total     int64     `json:"total"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func dto(p domain.Payment) PaymentDTO {
	return PaymentDTO{p.ID, p.OrderID, p.Total, p.Status, p.CreatedAt}
}
func (h *Handler) Routes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/payments", h.pay)
	m.HandleFunc("GET /api/v1/payments/{id}", h.get)
}
func (h *Handler) pay(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OrderID string `json:"order_id"`
		Total   int64  `json:"total"`
		Token   string `json:"token"`
	}
	if !web.Decode(w, r, &in) {
		return
	}
	p, err := h.service.Pay(r.Context(), in.OrderID, in.Total, in.Token)
	if errors.Is(err, domain.ErrInvalid) {
		web.Error(w, 422, "invalid_payment", err.Error())
		return
	}
	if errors.Is(err, domain.ErrConflict) {
		web.Error(w, 409, "payment_conflict", err.Error())
		return
	}
	if err != nil {
		web.Error(w, 503, "payment_unavailable", "Payment could not finish")
		return
	}
	w.Header().Set("Location", "/api/v1/payments/"+p.ID)
	if p.Status == "Declined" {
		web.JSON(w, 402, struct {
			Code    string     `json:"code"`
			Message string     `json:"message"`
			Payment PaymentDTO `json:"payment"`
		}{"payment_declined", "Demo provider declined payment", dto(p)})
		return
	}
	web.JSON(w, 201, dto(p))
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, err := h.service.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		web.Error(w, 404, "not_found", err.Error())
		return
	}
	if err != nil {
		web.Error(w, 500, "internal_error", "Could not read payment")
		return
	}
	web.JSON(w, 200, dto(p))
}
