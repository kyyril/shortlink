package link

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/auth"
	"github.com/kyyril/shortlink/internal/httpx"
)

// LinkService defines the operations required by the HTTP handler.
type LinkService interface {
	Create(ctx context.Context, in CreateInput) (Link, error)
	Get(ctx context.Context, userID uuid.UUID, code string) (Link, error)
	Resolve(ctx context.Context, code string) (Link, error)
	List(ctx context.Context, userID uuid.UUID, limit int, cursor *httpx.Cursor) ([]Link, *httpx.Cursor, error)
	Delete(ctx context.Context, userID uuid.UUID, code string) error
}

// Handler handles HTTP requests for the links domain.
type Handler struct {
	svc     LinkService
	baseURL string // e.g. "https://short.ly" — used to build short_url
}

// NewHandler returns a new Handler.
func NewHandler(svc LinkService, baseURL string) *Handler {
	return &Handler{svc: svc, baseURL: baseURL}
}

// createRequest is the POST /v1/links request body.
type createRequest struct {
	URL        string     `json:"url"`
	CustomCode string     `json:"custom_code,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// linkResponse is the JSON representation of a Link returned to the client.
type linkResponse struct {
	ID        uuid.UUID  `json:"id"`
	Code      string     `json:"code"`
	ShortURL  string     `json:"short_url"`
	TargetURL string     `json:"target_url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// listResponse is the JSON envelope for GET /v1/links.
type listResponse struct {
	Links      []linkResponse `json:"links"`
	NextCursor *string        `json:"next_cursor,omitempty"`
}

// toLinkResponse maps a domain Link to a linkResponse.
func (h *Handler) toLinkResponse(l Link) linkResponse {
	return linkResponse{
		ID:        l.ID,
		Code:      l.Code,
		ShortURL:  fmt.Sprintf("%s/%s", h.baseURL, l.Code),
		TargetURL: l.TargetURL,
		ExpiresAt: l.ExpiresAt,
		CreatedAt: l.CreatedAt,
	}
}

// Create handles POST /v1/links.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing user in context")
		return
	}

	var req createRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	l, err := h.svc.Create(r.Context(), CreateInput{
		UserID:     userID,
		URL:        req.URL,
		CustomCode: req.CustomCode,
		ExpiresAt:  req.ExpiresAt,
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	_ = httpx.EncodeJSON(w, http.StatusCreated, h.toLinkResponse(l))
}

// List handles GET /v1/links.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing user in context")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		n, err := strconv.Atoi(limitStr)
		if err != nil || n < 1 || n > 100 {
			httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid_param", "limit must be an integer between 1 and 100")
			return
		}
		limit = n
	}

	var cursor *httpx.Cursor
	if cursorStr := r.URL.Query().Get("cursor"); cursorStr != "" {
		c, err := httpx.DecodeCursor(cursorStr)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		cursor = &c
	}

	links, nextCursor, err := h.svc.List(r.Context(), userID, limit, cursor)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	resp := listResponse{
		Links: make([]linkResponse, len(links)),
	}
	for i, l := range links {
		resp.Links[i] = h.toLinkResponse(l)
	}

	if nextCursor != nil {
		encoded := httpx.EncodeCursor(*nextCursor)
		resp.NextCursor = &encoded
	}

	_ = httpx.EncodeJSON(w, http.StatusOK, resp)
}

// Get handles GET /v1/links/{code}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing user in context")
		return
	}

	code := r.PathValue("code")

	l, err := h.svc.Get(r.Context(), userID, code)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	_ = httpx.EncodeJSON(w, http.StatusOK, h.toLinkResponse(l))
}

// Delete handles DELETE /v1/links/{code}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing user in context")
		return
	}

	code := r.PathValue("code")

	if err := h.svc.Delete(r.Context(), userID, code); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Redirect handles GET /{code} — the public redirect endpoint.
func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	l, err := h.svc.Resolve(r.Context(), code)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	http.Redirect(w, r, l.TargetURL, http.StatusFound)
}
