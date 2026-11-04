package httpx_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kyyril/shortlink/internal/httpx"
)

type samplePayload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestDecodeJSON_ValidPayload_Succeeds(t *testing.T) {
	body := strings.NewReader(`{"name":"alice","age":30}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	rec := httptest.NewRecorder()

	var payload samplePayload
	if err := httpx.DecodeJSON(rec, req, &payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if payload.Name != "alice" || payload.Age != 30 {
		t.Errorf("decoded = %+v, want {name:alice, age:30}", payload)
	}
}

func TestDecodeJSON_UnknownField_ReturnsError(t *testing.T) {
	body := strings.NewReader(`{"name":"alice","age":30,"extra":"not_allowed"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	rec := httptest.NewRecorder()

	var payload samplePayload
	err := httpx.DecodeJSON(rec, req, &payload)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}

	if !errors.Is(err, httpx.ErrUnknownField) {
		t.Errorf("expected ErrUnknownField, got: %v", err)
	}
}

func TestDecodeJSON_OversizedBody_ReturnsError(t *testing.T) {
	// Create a payload larger than 1 MB (MaxBodyBytes)
	largeString := strings.Repeat("a", httpx.MaxBodyBytes+100)
	body := strings.NewReader(`{"name":"` + largeString + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	rec := httptest.NewRecorder()

	var payload samplePayload
	err := httpx.DecodeJSON(rec, req, &payload)
	if err == nil {
		t.Fatal("expected error for oversized body, got nil")
	}

	if !errors.Is(err, httpx.ErrBodyTooLarge) {
		t.Errorf("expected ErrBodyTooLarge, got: %v", err)
	}
}

func TestDecodeJSON_InvalidJSON_ReturnsError(t *testing.T) {
	body := strings.NewReader(`{invalid json}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	rec := httptest.NewRecorder()

	var payload samplePayload
	err := httpx.DecodeJSON(rec, req, &payload)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}

	if !errors.Is(err, httpx.ErrInvalidJSON) {
		t.Errorf("expected ErrInvalidJSON, got: %v", err)
	}
}

func TestDecodeJSON_MultipleDocuments_ReturnsError(t *testing.T) {
	body := strings.NewReader(`{"name":"alice"}{"name":"bob"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	rec := httptest.NewRecorder()

	var payload samplePayload
	err := httpx.DecodeJSON(rec, req, &payload)
	if err == nil {
		t.Fatal("expected error for multiple JSON docs, got nil")
	}

	if !errors.Is(err, httpx.ErrInvalidJSON) {
		t.Errorf("expected ErrInvalidJSON, got: %v", err)
	}
}

func TestEncodeJSON_ValidData_EncodesWithContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := map[string]string{"message": "success"}

	if err := httpx.EncodeJSON(rec, http.StatusCreated, payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	expected := `{"message":"success"}`
	if !bytes.Contains(rec.Body.Bytes(), []byte(expected)) {
		t.Errorf("body = %q, want to contain %q", rec.Body.String(), expected)
	}
}
