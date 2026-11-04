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

// LinkRepo implements link.Repository using pgxpool and generated sqlc queries.
type LinkRepo struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewLinkRepo returns a new LinkRepo.
func NewLinkRepo(pool *pgxpool.Pool) *LinkRepo {
	return &LinkRepo{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// Insert persists a new link. Translates unique-code constraint (23505) to link.ErrCodeTaken.
func (r *LinkRepo) Insert(ctx context.Context, l link.Link) error {
	params := sqlc.InsertLinkParams{
		ID:        l.ID,
		UserID:    l.UserID,
		Code:      l.Code,
		TargetUrl: l.TargetURL,
		ExpiresAt: l.ExpiresAt,
		CreatedAt: l.CreatedAt,
	}

	err := r.queries.InsertLink(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return link.ErrCodeTaken
		}
		return fmt.Errorf("insert link: %w", err)
	}

	return nil
}

// GetByCode returns a link by its short code. Returns link.ErrNotFound when missing.
func (r *LinkRepo) GetByCode(ctx context.Context, code string) (link.Link, error) {
	row, err := r.queries.GetLinkByCode(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return link.Link{}, link.ErrNotFound
		}
		return link.Link{}, fmt.Errorf("get link by code: %w", err)
	}

	return toLink(row), nil
}

// GetOwned returns a link only when it belongs to userID. Returns link.ErrNotFound otherwise.
func (r *LinkRepo) GetOwned(ctx context.Context, code string, userID uuid.UUID) (link.Link, error) {
	row, err := r.queries.GetOwnedLink(ctx, sqlc.GetOwnedLinkParams{
		Code:   code,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return link.Link{}, link.ErrNotFound
		}
		return link.Link{}, fmt.Errorf("get owned link: %w", err)
	}

	return toLink(row), nil
}

// ListByUser returns up to limit links for a user ordered newest first.
// If after is nil the first page is returned; otherwise results start after that cursor position.
func (r *LinkRepo) ListByUser(ctx context.Context, userID uuid.UUID, after *link.Cursor, limit int) ([]link.Link, error) {
	pageSize := int32(limit) //nolint:gosec // limit is validated to be small before this call

	var rows []sqlc.Link
	var err error

	if after == nil {
		rows, err = r.queries.ListLinksFirstPage(ctx, sqlc.ListLinksFirstPageParams{
			UserID:   userID,
			PageSize: pageSize,
		})
	} else {
		rows, err = r.queries.ListLinksAfter(ctx, sqlc.ListLinksAfterParams{
			UserID:         userID,
			AfterCreatedAt: after.CreatedAt,
			AfterID:        after.ID,
			PageSize:       pageSize,
		})
	}

	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}

	links := make([]link.Link, len(rows))
	for i, row := range rows {
		links[i] = toLink(row)
	}

	return links, nil
}

// DeleteOwned deletes a link only when it belongs to userID.
func (r *LinkRepo) DeleteOwned(ctx context.Context, code string, userID uuid.UUID) error {
	err := r.queries.DeleteOwnedLink(ctx, sqlc.DeleteOwnedLinkParams{
		Code:   code,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("delete owned link: %w", err)
	}

	return nil
}

// toLink maps a sqlc.Link to the domain link.Link type.
func toLink(row sqlc.Link) link.Link {
	return link.Link{
		ID:        row.ID,
		UserID:    row.UserID,
		Code:      row.Code,
		TargetURL: row.TargetUrl,
		ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt,
	}
}
