package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/auth"
	"github.com/kyyril/shortlink/internal/httpx"
)

type fakeAuthService struct {
	registerFunc func(ctx context.Context, email, password string) (auth.User, error)
	loginFunc    func(ctx context.Context, email, password string) (string, error)
}

func (f *fakeAuthService) Register(ctx context.Context, email, password string) (auth.User, error) {
	if f.registerFunc != nil {
		return f.registerFunc(ctx, email, password)
	}
	return auth.User{}, nil
}

func (f *fakeAuthService) Login(ctx context.Context, email, password string) (string, error) {
	if f.loginFunc != nil {
		return f.loginFunc(ctx, email, password)
	}
	return "", nil
}

// AC-1: Given no account, when I register with a valid email and password,
// then I get 201 and no password hash in the response.
func TestHandler_Register_ValidInput_AC1(t *testing.T) {
	userID := uuid.New()
	createdAt := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

	svc := &fakeAuthService{
		registerFunc: func(ctx context.Context, email, password string) (auth.User, error) {
			return auth.User{
				ID:           userID,
				Email:        email,
				PasswordHash: "$2a$12$supersecrethashneverleakthis",
				CreatedAt:    createdAt,
			}, nil
		},
	}

	h := auth.NewHandler(svc)

	body := strings.NewReader(`{"email":"newuser@example.com","password":"password123"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", body)
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusCreated)
	}

	rawBody := rec.Body.String()
	if strings.Contains(rawBody, "supersecrethash") || strings.Contains(rawBody, "password_hash") {
		t.Errorf("response body leaks password hash: %s", rawBody)
	}

	var resp auth.RegisterResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID != userID {
		t.Errorf("resp.ID = %v, want %v", resp.ID, userID)
	}
	if resp.Email != "newuser@example.com" {
		t.Errorf("resp.Email = %q, want 'newuser@example.com'", resp.Email)
	}
}

// AC-2: Given an existing email in different letter case, when I register,
// then I get 409 with code email_taken.
func TestHandler_Register_EmailTaken_AC2(t *testing.T) {
	svc := &fakeAuthService{
		registerFunc: func(ctx context.Context, email, password string) (auth.User, error) {
			return auth.User{}, auth.ErrEmailTaken
		},
	}

	h := auth.NewHandler(svc)

	body := strings.NewReader(`{"email":"existing@example.com","password":"password123"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", body)
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusConflict)
	}

	var p httpx.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("failed to decode problem JSON: %v", err)
	}

	if p.Code != "email_taken" {
		t.Errorf("problem code = %q, want 'email_taken'", p.Code)
	}
}

// AC-3: Given wrong password or unknown email, when I log in,
// then I get 401 with the same message for both cases.
func TestHandler_Login_InvalidCredentials_AC3(t *testing.T) {
	tests := []struct {
		name       string
		loginError error
	}{
		{
			name:       "WrongPassword",
			loginError: auth.ErrInvalidCredentials,
		},
		{
			name:       "UnknownEmail",
			loginError: auth.ErrInvalidCredentials,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeAuthService{
				loginFunc: func(ctx context.Context, email, password string) (string, error) {
					return "", tc.loginError
				},
			}

			h := auth.NewHandler(svc)

			body := strings.NewReader(`{"email":"user@example.com","password":"badpassword"}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", body)
			rec := httptest.NewRecorder()

			h.Login(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status code = %d, want %d", rec.Code, http.StatusUnauthorized)
			}

			var p httpx.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("failed to decode problem JSON: %v", err)
			}

			if p.Code != "invalid_credentials" {
				t.Errorf("problem code = %q, want 'invalid_credentials'", p.Code)
			}
			if p.Detail != "invalid email or password" {
				t.Errorf("problem detail = %q, want 'invalid email or password'", p.Detail)
			}
		})
	}
}

func TestHandler_Login_Success_ReturnsAccessToken(t *testing.T) {
	const dummyToken = "jwt.token.string"
	svc := &fakeAuthService{
		loginFunc: func(ctx context.Context, email, password string) (string, error) {
			return dummyToken, nil
		},
	}

	h := auth.NewHandler(svc)

	body := strings.NewReader(`{"email":"user@example.com","password":"correctpassword"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", body)
	rec := httptest.NewRecorder()

	h.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp auth.LoginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.AccessToken != dummyToken {
		t.Errorf("access_token = %q, want %q", resp.AccessToken, dummyToken)
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want 'Bearer'", resp.TokenType)
	}
}
