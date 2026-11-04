package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Problem implements RFC 9457 Problem Details for HTTP APIs.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Detail    string `json:"detail"`
	RequestID string `json:"request_id,omitempty"`
}

// ErrorCoder allows domain errors to specify HTTP status and problem codes
// without httpx importing domain packages.
type ErrorCoder interface {
	error
	StatusCode() int
	ErrorCode() string
}

// ErrorMapper is a function that maps an error to an HTTP status, problem code, and detail.
// It returns false if the error is not recognized.
type ErrorMapper func(err error) (status int, code string, detail string, ok bool)

var customMappers []ErrorMapper

// RegisterErrorMapper registers a custom function to map domain errors to problem details.
func RegisterErrorMapper(m ErrorMapper) {
	customMappers = append(customMappers, m)
}

// WriteProblem writes an RFC 9457 problem JSON response.
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	reqID := GetRequestID(r.Context())

	title := http.StatusText(status)
	if title == "" {
		title = "Error"
	}

	p := Problem{
		Type:      "about:blank",
		Title:     title,
		Status:    status,
		Code:      code,
		Detail:    detail,
		RequestID: reqID,
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

// WriteError inspects err and writes the corresponding problem JSON response.
// Unknown errors fall back to 500 internal error.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}

	// 1. Check custom mappers
	for _, mapper := range customMappers {
		if status, code, detail, ok := mapper(err); ok {
			WriteProblem(w, r, status, code, detail)
			return
		}
	}

	// 2. Check if error implements ErrorCoder
	var ec ErrorCoder
	if errors.As(err, &ec) {
		WriteProblem(w, r, ec.StatusCode(), ec.ErrorCode(), ec.Error())
		return
	}

	// 3. Built-in error mappings
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		WriteProblem(w, r, http.StatusRequestEntityTooLarge, "body_too_large", "request body exceeds 1 MB limit")
		return
	}

	if errors.Is(err, ErrBodyTooLarge) {
		WriteProblem(w, r, http.StatusRequestEntityTooLarge, "body_too_large", "request body exceeds 1 MB limit")
		return
	}

	if errors.Is(err, ErrInvalidJSON) || errors.Is(err, ErrUnknownField) {
		WriteProblem(w, r, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	// Fallback to 500 Internal Server Error
	WriteProblem(w, r, http.StatusInternalServerError, "internal", "an unexpected error occurred")
}
