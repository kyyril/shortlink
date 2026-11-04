package httpx_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kyyril/shortlink/internal/httpx"
)

// AC-17: Given more requests than the limit from one IP, extra requests get 429 with Retry-After.
func TestRateLimiter_ExceedLimit_Returns429WithRetryAfter(t *testing.T) {
	// 2 rps, burst 2
	rl := httpx.NewRateLimiter(2, 2, time.Minute, time.Minute, false)
	defer func() { _ = rl.Close(context.Background()) }()

	handler := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request 1: OK
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "192.0.2.1:1234"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("req 1 status = %d, want 200", rec1.Code)
	}

	// Request 2: OK (burst exhausted)
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "192.0.2.1:1234"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("req 2 status = %d, want 200", rec2.Code)
	}

	// Request 3: Over limit -> 429 with Retry-After
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "192.0.2.1:1234"
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusTooManyRequests {
		t.Errorf("req 3 status = %d, want 429", rec3.Code)
	}
	retryAfter := rec3.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("expected Retry-After header to be set")
	}

	// Request from a different IP is unaffected
	reqOther := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqOther.RemoteAddr = "198.51.100.2:5678"
	recOther := httptest.NewRecorder()
	handler.ServeHTTP(recOther, reqOther)
	if recOther.Code != http.StatusOK {
		t.Errorf("other IP request status = %d, want 200", recOther.Code)
	}
}

func TestRateLimiter_ClientIP_TrustForwarded(t *testing.T) {
	// Not trusting forwarded headers
	rlNoTrust := httpx.NewRateLimiter(10, 10, time.Minute, time.Minute, false)
	defer func() { _ = rlNoTrust.Close(context.Background()) }()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.195:443"
	req.Header.Set("X-Forwarded-For", "198.51.100.1, 10.0.0.1")

	ip := rlNoTrust.ClientIP(req)
	if ip != "203.0.113.195" {
		t.Errorf("untrusted IP = %q, want %q", ip, "203.0.113.195")
	}

	// Trusting forwarded headers
	rlTrust := httpx.NewRateLimiter(10, 10, time.Minute, time.Minute, true)
	defer func() { _ = rlTrust.Close(context.Background()) }()

	ipTrusted := rlTrust.ClientIP(req)
	if ipTrusted != "198.51.100.1" {
		t.Errorf("trusted IP = %q, want %q", ipTrusted, "198.51.100.1")
	}
}

func TestRateLimiter_CleanupIdle(t *testing.T) {
	// idleTTL = 10ms
	rl := httpx.NewRateLimiter(10, 10, 10*time.Millisecond, time.Hour, false)
	defer func() { _ = rl.Close(context.Background()) }()

	handler := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.55:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Wait past idleTTL
	time.Sleep(20 * time.Millisecond)

	// Run manual cleanup
	rl.CleanupIdle()

	// New request creates fresh limiter
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec2.Code)
	}
}
