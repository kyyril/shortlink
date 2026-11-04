package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// MaxBodyBytes is the maximum allowed request body size (1 MB).
const MaxBodyBytes = 1 << 20

// Standard errors returned by DecodeJSON.
var (
	ErrBodyTooLarge = errors.New("request body too large")
	ErrInvalidJSON  = errors.New("invalid JSON")
	ErrUnknownField = errors.New("unknown field in JSON")
)

// DecodeJSON reads up to 1 MB from r.Body, rejects unknown fields, and unmarshals into dst.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return fmt.Errorf("%w: max size is %d bytes", ErrBodyTooLarge, MaxBodyBytes)
		}

		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			return fmt.Errorf("%w at offset %d: %s", ErrInvalidJSON, syntaxErr.Offset, syntaxErr.Error())
		}

		var unmarshalTypeErr *json.UnmarshalTypeError
		if errors.As(err, &unmarshalTypeErr) {
			return fmt.Errorf("%w: expected %s for field %q, got %s", ErrInvalidJSON, unmarshalTypeErr.Type, unmarshalTypeErr.Field, unmarshalTypeErr.Value)
		}

		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return fmt.Errorf("%w: %s", ErrUnknownField, fieldName)
		}

		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%w: body is empty", ErrInvalidJSON)
		}

		return fmt.Errorf("%w: %s", ErrInvalidJSON, err.Error())
	}

	// Ensure there is only one JSON document in the body
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: body contains multiple JSON values", ErrInvalidJSON)
	}

	return nil
}

// EncodeJSON writes data as JSON with the given status code.
func EncodeJSON(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
