package httpx_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kyyril/shortlink/internal/httpx"
)

func TestRecover_PanicInHandler_Returns500ProblemJSONAndServerContinues(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("unexpected explosion")
	})

	mw := httpx.Chain(panicHandler, httpx.RequestID, httpx.Recover(logger))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	// 1. Must not panic
	mw.ServeHTTP(rec, req)

	// 2. Status must be 500
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	// 3. Content-Type must be application/problem+json
	ct := rec.Header().Get("Content-Type")
	if ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/problem+json")
	}

	// 4. Response body must match problem JSON shape
	var prob httpx.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &prob); err != nil {
		t.Fatalf("failed to parse problem JSON: %v, body: %s", err, rec.Body.String())
	}

	if prob.Status != http.StatusInternalServerError {
		t.Errorf("prob.Status = %d, want %d", prob.Status, http.StatusInternalServerError)
	}
	if prob.Code != "internal" {
		t.Errorf("prob.Code = %q, want %q", prob.Code, "internal")
	}
	if prob.Detail != "an unexpected error occurred" {
		t.Errorf("prob.Detail = %q, want %q", prob.Detail, "an unexpected error occurred")
	}
	if prob.RequestID == "" {
		t.Error("prob.RequestID is empty, expected request ID from context")
	}

	// 5. Subsequent request must still work (server keeps running)
	normalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mwNormal := httpx.Chain(normalHandler, httpx.RequestID, httpx.Recover(logger))

	req2 := httptest.NewRequest(http.MethodGet, "/normal", nil)
	rec2 := httptest.NewRecorder()
	mwNormal.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("second request status = %d, want %d", rec2.Code, http.StatusOK)
	}
}
