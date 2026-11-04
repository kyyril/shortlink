package link

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// IdempotencyRecord represents a cached HTTP response keyed by an Idempotency-Key.
type IdempotencyRecord struct {
	Key          string
	UserID       uuid.UUID
	RequestHash  string
	StatusCode   int
	ResponseBody string
	CreatedAt    time.Time
}

// IdempotencyStore defines storage operations for idempotency records.
type IdempotencyStore interface {
	Get(ctx context.Context, key string, userID uuid.UUID) (*IdempotencyRecord, error)
	Save(ctx context.Context, rec IdempotencyRecord) error
}
