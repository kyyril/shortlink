package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kyyril/shortlink/internal/link"
	"github.com/kyyril/shortlink/internal/store/sqlc"
)

// IdempotencyRepo implements link.IdempotencyStore.
type IdempotencyRepo struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewIdempotencyRepo creates a new IdempotencyRepo.
func NewIdempotencyRepo(pool *pgxpool.Pool) *IdempotencyRepo {
	return &IdempotencyRepo{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// Get finds an idempotency record by key and userID. Returns nil, nil if not found.
func (r *IdempotencyRepo) Get(ctx context.Context, key string, userID uuid.UUID) (*link.IdempotencyRecord, error) {
	row, err := r.queries.GetIdempotencyKey(ctx, sqlc.GetIdempotencyKeyParams{
		Key:    key,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get idempotency key: %w", err)
	}

	return &link.IdempotencyRecord{
		Key:          row.Key,
		UserID:       row.UserID,
		RequestHash:  row.RequestHash,
		StatusCode:   int(row.StatusCode),
		ResponseBody: row.ResponseBody,
		CreatedAt:    row.CreatedAt,
	}, nil
}

// Save persists an idempotency record. Returns link.ErrCodeTaken or generic on unique constraint.
func (r *IdempotencyRepo) Save(ctx context.Context, rec link.IdempotencyRecord) error {
	err := r.queries.InsertIdempotencyKey(ctx, sqlc.InsertIdempotencyKeyParams{
		Key:          rec.Key,
		UserID:       rec.UserID,
		RequestHash:  rec.RequestHash,
		StatusCode:   int32(rec.StatusCode),
		ResponseBody: rec.ResponseBody,
		CreatedAt:    rec.CreatedAt,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return errors.New("concurrent idempotency key insert conflict")
		}
		return fmt.Errorf("insert idempotency key: %w", err)
	}
	return nil
}
