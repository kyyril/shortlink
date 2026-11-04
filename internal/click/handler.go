package click

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/auth"
	"github.com/kyyril/shortlink/internal/httpx"
)

// StatsService provides the GetStats operation.
type StatsService interface {
	GetStats(ctx context.Context, userID uuid.UUID, code string) (Stats, error)
}

// Handler handles HTTP requests for click stats.
type Handler struct {
	svc StatsService
}

// NewHandler creates a new click stats Handler.
func NewHandler(svc StatsService) *Handler {
	return &Handler{svc: svc}
}

// GetStats handles GET /v1/links/{code}/stats.
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing user in context")
		return
	}

	code := r.PathValue("code")
	stats, err := h.svc.GetStats(r.Context(), userID, code)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	_ = httpx.EncodeJSON(w, http.StatusOK, stats)
}
