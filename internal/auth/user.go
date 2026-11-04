package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

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

var (
	ErrEmailTaken         = &codedError{msg: "email already taken", status: 409, code: "email_taken"}
	ErrInvalidCredentials = &codedError{msg: "invalid email or password", status: 401, code: "invalid_credentials"}
	ErrInvalidEmail       = &codedError{msg: "invalid email address", status: 422, code: "invalid_email"}
	ErrShortPassword      = &codedError{msg: "password must be at least 8 characters", status: 422, code: "short_password"}
	ErrTokenInvalid       = &codedError{msg: "invalid token", status: 401, code: "unauthorized"}
	ErrTokenExpired       = &codedError{msg: "token expired", status: 401, code: "unauthorized"}
	ErrUserNotFound       = &codedError{msg: "user not found", status: 404, code: "not_found"}
)

// User represents the authenticated user entity.
type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// Repository defines data access methods needed by the auth domain.
type Repository interface {
	Create(ctx context.Context, u User) error
	FindByEmail(ctx context.Context, email string) (User, error)
	FindByID(ctx context.Context, id uuid.UUID) (User, error)
}
