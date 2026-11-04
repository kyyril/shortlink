package httpx_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kyyril/shortlink/internal/httpx"
)

type customDomainError struct {
	status int
	code   string
	msg    string
}

func (e *customDomainError) Error() string     { return e.msg }
func (e *customDomainError) StatusCode() int   { return e.status }
func (e *customDomainError) ErrorCode() string { return e.code }

func TestWriteProblem_FormatsRFC9457(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := httpx.WithRequestID(req.Context(), "req-xyz-987")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	httpx.WriteProblem(rec, req, http.StatusConflict, "code_taken", "custom code already in use")

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}

	var p httpx.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("failed to decode problem JSON: %v", err)
	}

	if p.Type != "about:blank" {
		t.Errorf("type = %q, want about:blank", p.Type)
	}
	if p.Title != "Conflict" {
		t.Errorf("title = %q, want Conflict", p.Title)
	}
	if p.Status != http.StatusConflict {
		t.Errorf("status = %d, want %d", p.Status, http.StatusConflict)
	}
	if p.Code != "code_taken" {
		t.Errorf("code = %q, want code_taken", p.Code)
	}
	if p.Detail != "custom code already in use" {
		t.Errorf("detail = %q, want 'custom code already in use'", p.Detail)
	}
	if p.RequestID != "req-xyz-987" {
		t.Errorf("request_id = %q, want req-xyz-987", p.RequestID)
	}
}

func TestWriteError_MapsErrorsCorrectly(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "CustomCodedError",
			err:        &customDomainError{status: 404, code: "not_found", msg: "link not found"},
			wantStatus: 404,
			wantCode:   "not_found",
		},
		{
			name:       "BodyTooLargeError",
			err:        httpx.ErrBodyTooLarge,
			wantStatus: 413,
			wantCode:   "body_too_large",
		},
		{
			name:       "UnknownFieldError",
			err:        httpx.ErrUnknownField,
			wantStatus: 400,
			wantCode:   "bad_request",
		},
		{
			name:       "UnknownGenericError_MapsTo500Internal",
			err:        errors.New("db connection dropped unexpectedly"),
			wantStatus: 500,
			wantCode:   "internal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			httpx.WriteError(rec, req, tc.err)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}

			var p httpx.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("failed to decode problem: %v", err)
			}

			if p.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", p.Code, tc.wantCode)
			}
		})
	}
}
