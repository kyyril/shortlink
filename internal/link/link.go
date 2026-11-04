package link

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// codedError is a domain error that carries an HTTP status code and a stable error code.
type codedError struct {
	msg    string
	status int
	code   string
}

func (e *codedError) Error() string {
	return e.msg
}

func (e *codedError) StatusCode() int {
	return e.status
}

func (e *codedError) ErrorCode() string {
	return e.code
}

// Sentinel errors for the link domain.
var (
	ErrNotFound      = &codedError{msg: "link not found", status: 404, code: "not_found"}
	ErrCodeTaken     = &codedError{msg: "code already in use", status: 409, code: "code_taken"}
	ErrExpired       = &codedError{msg: "link has expired", status: 410, code: "link_expired"}
	ErrInvalidURL    = &codedError{msg: "invalid url", status: 422, code: "invalid_url"}
	ErrInvalidCode   = &codedError{msg: "invalid custom code", status: 422, code: "invalid_code"}
	ErrInvalidExpiry = &codedError{msg: "expires_at must be in the future", status: 422, code: "invalid_expiry"}
	ErrCodeExhausted = &codedError{msg: "could not generate a unique code, try again", status: 503, code: "code_exhausted"}
)

// Link is the core domain entity.
type Link struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Code      string
	TargetURL string
	ExpiresAt *time.Time
	CreatedAt time.Time
}

// Cursor holds the position for keyset pagination over a user's links.
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// Repository defines persistence operations for the link domain.
// It is implemented in internal/store.
type Repository interface {
	// Insert persists a new link. Returns ErrCodeTaken on unique-code violation.
	Insert(ctx context.Context, l Link) error

	// GetByCode returns a link by its short code. Returns ErrNotFound when missing.
	GetByCode(ctx context.Context, code string) (Link, error)

	// GetOwned returns a link only when it belongs to userID. Returns ErrNotFound otherwise.
	GetOwned(ctx context.Context, code string, userID uuid.UUID) (Link, error)

	// ListByUser returns up to limit links for a user ordered by (created_at DESC, id DESC).
	// If after is nil the first page is returned; otherwise results start after that cursor.
	ListByUser(ctx context.Context, userID uuid.UUID, after *Cursor, limit int) ([]Link, error)

	// DeleteOwned deletes a link only when it belongs to userID.
	DeleteOwned(ctx context.Context, code string, userID uuid.UUID) error
}
