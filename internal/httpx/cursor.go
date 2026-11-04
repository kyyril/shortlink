package httpx

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Cursor is an opaque keyset-pagination cursor encoding (created_at, id).
type Cursor struct {
	CreatedAt time.Time `json:"t"`
	ID        uuid.UUID `json:"i"`
}

// errInvalidCursor is returned when a cursor string cannot be decoded.
var ErrInvalidCursor = &httpError{
	msg:    "invalid or tampered cursor",
	status: http.StatusBadRequest,
	code:   "invalid_cursor",
}

type httpError struct {
	msg    string
	status int
	code   string
}

func (e *httpError) Error() string {
	return e.msg
}

func (e *httpError) StatusCode() int {
	return e.status
}

func (e *httpError) ErrorCode() string {
	return e.code
}

// EncodeCursor serialises c to a base64url string safe for use as a query parameter.
func EncodeCursor(c Cursor) string {
	b, _ := json.Marshal(c) // Cursor only contains time.Time and uuid.UUID; marshal never fails
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses an encoded cursor string.
// Returns ErrInvalidCursor for empty, truncated, or tampered input.
func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, ErrInvalidCursor
	}

	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}

	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return Cursor{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}

	// Reject zero values — a valid cursor must have a non-zero time and non-nil UUID.
	if c.CreatedAt.IsZero() || c.ID == (uuid.UUID{}) {
		return Cursor{}, ErrInvalidCursor
	}

	return c, nil
}
