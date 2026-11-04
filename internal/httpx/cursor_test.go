package httpx_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/httpx"
)

func TestCursor_RoundTrip(t *testing.T) {
	original := httpx.Cursor{
		CreatedAt: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
		ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
	}

	encoded := httpx.EncodeCursor(original)
	got, err := httpx.DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeCursor() error: %v", err)
	}

	if !got.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, original.CreatedAt)
	}
	if got.ID != original.ID {
		t.Errorf("ID = %v, want %v", got.ID, original.ID)
	}
}

func TestCursor_EmptyString_ReturnsErrInvalidCursor(t *testing.T) {
	_, err := httpx.DecodeCursor("")
	if !errors.Is(err, httpx.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor, got: %v", err)
	}
}

func TestCursor_TruncatedBase64_ReturnsErrInvalidCursor(t *testing.T) {
	c := httpx.Cursor{
		CreatedAt: time.Now().UTC(),
		ID:        uuid.New(),
	}
	encoded := httpx.EncodeCursor(c)

	// Truncate to half
	truncated := encoded[:len(encoded)/2]
	_, err := httpx.DecodeCursor(truncated)
	if !errors.Is(err, httpx.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor for truncated cursor, got: %v", err)
	}
}

func TestCursor_TamperedBytes_ReturnsErrInvalidCursor(t *testing.T) {
	c := httpx.Cursor{
		CreatedAt: time.Now().UTC(),
		ID:        uuid.New(),
	}
	encoded := httpx.EncodeCursor(c)

	// Flip last character to something else
	tampered := encoded[:len(encoded)-1] + "!"
	_, err := httpx.DecodeCursor(tampered)
	if !errors.Is(err, httpx.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor for tampered cursor, got: %v", err)
	}
}

func TestCursor_InvalidJSON_ReturnsErrInvalidCursor(t *testing.T) {
	import64 := "aW52YWxpZA" // base64url of "invalid"
	_, err := httpx.DecodeCursor(import64)
	if !errors.Is(err, httpx.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor for invalid JSON, got: %v", err)
	}
}
