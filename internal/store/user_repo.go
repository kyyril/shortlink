package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kyyril/shortlink/internal/auth"
	"github.com/kyyril/shortlink/internal/store/sqlc"
)

// UserRepo implements auth.Repository using pgxpool and generated sqlc queries.
type UserRepo struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewUserRepo returns a new UserRepo.
func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// Create inserts a new user into the database.
// Translates PostgreSQL unique constraint violation (23505) to auth.ErrEmailTaken.
func (r *UserRepo) Create(ctx context.Context, u auth.User) error {
	params := sqlc.CreateUserParams{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt,
	}

	_, err := r.queries.CreateUser(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return auth.ErrEmailTaken
		}
		return fmt.Errorf("create user: %w", err)
	}

	return nil
}

// FindByEmail searches for a user by email (case-insensitive due to citext).
func (r *UserRepo) FindByEmail(ctx context.Context, email string) (auth.User, error) {
	row, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.User{}, auth.ErrUserNotFound
		}
		return auth.User{}, fmt.Errorf("find user by email: %w", err)
	}

	return auth.User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		CreatedAt:    row.CreatedAt,
	}, nil
}

// FindByID retrieves a user by UUID.
func (r *UserRepo) FindByID(ctx context.Context, id uuid.UUID) (auth.User, error) {
	row, err := r.queries.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.User{}, auth.ErrUserNotFound
		}
		return auth.User{}, fmt.Errorf("find user by id: %w", err)
	}

	return auth.User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		CreatedAt:    row.CreatedAt,
	}, nil
}
