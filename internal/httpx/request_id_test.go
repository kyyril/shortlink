package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kyyril/shortlink/internal/httpx"
)

func TestRequestID_MissingHeader_GeneratesNewUUID(t *testing.T) {
	var capturedID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = httpx.GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	mw := httpx.RequestID(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if capturedID == "" {
		t.Fatal("expected generated request ID in context, got empty string")
	}

	headerID := rec.Header().Get(httpx.HeaderRequestID)
	if headerID != capturedID {
		t.Errorf("response header X-Request-ID = %q, want %q", headerID, capturedID)
	}
}

func TestRequestID_ExistingHeader_PropagatesID(t *testing.T) {
	const incomingID = "custom-client-request-id-1234"

	var capturedID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = httpx.GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	mw := httpx.RequestID(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(httpx.HeaderRequestID, incomingID)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if capturedID != incomingID {
		t.Errorf("context request ID = %q, want %q", capturedID, incomingID)
	}

	headerID := rec.Header().Get(httpx.HeaderRequestID)
	if headerID != incomingID {
		t.Errorf("response header X-Request-ID = %q, want %q", headerID, incomingID)
	}
}
