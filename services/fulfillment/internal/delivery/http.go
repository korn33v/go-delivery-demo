package delivery

import (
	"errors"
	"godelivery/internal/platform/web"
	"godelivery/services/fulfillment/internal/domain"
	"godelivery/services/fulfillment/internal/usecase"
	"net/http"
	"time"
)

type Handler struct{ service *usecase.Service }

func New(s *usecase.Service) *Handler { return &Handler{s} }

type FulfillmentDTO struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"order_id"`
	Status    string    `json:"status"`
	Sequence  int       `json:"sequence"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func dto(f domain.Fulfillment) FulfillmentDTO {
	return FulfillmentDTO{f.ID, f.OrderID, f.Status, f.Sequence, f.CreatedAt, f.UpdatedAt}
}
func (h *Handler) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/fulfillments/{id}", h.get)
	m.HandleFunc("POST /api/v1/fulfillments/{id}/advance", h.advance)
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	f, err := h.service.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		h.failure(w, err)
		return
	}
	w.Header().Set("X-Workflow-Traceparent", f.TraceHeaders["traceparent"])
	web.JSON(w, 200, dto(f))
}
func (h *Handler) advance(w http.ResponseWriter, r *http.Request) {
	f, err := h.service.Advance(r.Context(), r.PathValue("id"))
	if err != nil {
		h.failure(w, err)
		return
	}
	web.JSON(w, 200, dto(f))
}
func (h *Handler) failure(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		web.Error(w, 404, "not_found", err.Error())
		return
	}
	if errors.Is(err, domain.ErrCompleted) {
		web.Error(w, 409, "already_completed", err.Error())
		return
	}
	web.Error(w, 500, "internal_error", "Fulfillment operation failed")
}
