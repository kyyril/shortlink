package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/httpx"
)

// AuthService specifies the operations required by Handler.
type AuthService interface {
	Register(ctx context.Context, email, password string) (User, error)
	Login(ctx context.Context, email, password string) (string, error)
}

// Handler handles HTTP requests for user authentication.
type Handler struct {
	svc AuthService
}

// NewHandler returns a new authentication Handler.
func NewHandler(svc AuthService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRequest defines the input payload for POST /v1/auth/register.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterResponse defines the response payload for POST /v1/auth/register.
// Contains user metadata; never leaks password hash.
type RegisterResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// LoginRequest defines the input payload for POST /v1/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse defines the response payload for POST /v1/auth/login.
type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// Register handles user registration (POST /v1/auth/register).
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	user, err := h.svc.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	resp := RegisterResponse{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
	}

	_ = httpx.EncodeJSON(w, http.StatusCreated, resp)
}

// Login handles user authentication (POST /v1/auth/login).
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	token, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	resp := LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
	}

	_ = httpx.EncodeJSON(w, http.StatusOK, resp)
}

