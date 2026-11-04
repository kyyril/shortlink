package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// BcryptCost defines the hashing work factor required by the security spec.
const BcryptCost = 12

// TokenIssuer defines the iss claim in issued JWTs.
const TokenIssuer = "shortlink"

// Service provides authentication business logic: registration, login, and token handling.
type Service struct {
	repo      Repository
	jwtSecret []byte
	jwtTTL    time.Duration
	now       func() time.Time
}

// NewService creates and returns an authentication Service.
func NewService(repo Repository, jwtSecret string, jwtTTL time.Duration, now func() time.Time) *Service {
	return &Service{
		repo:      repo,
		jwtSecret: []byte(jwtSecret),
		jwtTTL:    jwtTTL,
		now:       now,
	}
}

// Register validates email and password, hashes the password with bcrypt cost 12,
// and saves the new user in the repository.
func (s *Service) Register(ctx context.Context, email, password string) (User, error) {
	email = strings.TrimSpace(email)
	if err := validateEmail(email); err != nil {
		return User{}, err
	}

	if len(password) < 8 {
		return User{}, ErrShortPassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}

	user := User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: string(hash),
		CreatedAt:    s.now(),
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return User{}, err
	}

	return user, nil
}

// Login authenticates a user by email and password, returning a signed JWT access token.
// Returns ErrInvalidCredentials if either email is not found or password does not match.
func (s *Service) Login(ctx context.Context, email, password string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return "", ErrInvalidCredentials
	}

	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return "", ErrInvalidCredentials
		}
		return "", fmt.Errorf("find user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	token, err := s.IssueToken(user)
	if err != nil {
		return "", fmt.Errorf("issue token: %w", err)
	}

	return token, nil
}

// IssueToken issues a new HS256 JWT access token for the given user.
func (s *Service) IssueToken(u User) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   u.ID.String(),
		Issuer:    TokenIssuer,
		IssuedAt:  jwt.NewNumericDate(s.now()),
		ExpiresAt: jwt.NewNumericDate(s.now().Add(s.jwtTTL)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return tokenStr, nil
}

// VerifyToken validates the token string and returns the authenticated user's UUID.
// Rejects tokens with wrong algorithm, expired tokens, or invalid signatures.
func (s *Service) VerifyToken(tokenStr string) (uuid.UUID, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}),
		jwt.WithIssuer(TokenIssuer),
		jwt.WithTimeFunc(s.now),
	)

	var claims jwt.RegisteredClaims
	token, err := parser.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (any, error) {
		return s.jwtSecret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return uuid.Nil, ErrTokenExpired
		}
		return uuid.Nil, ErrTokenInvalid
	}

	if !token.Valid {
		return uuid.Nil, ErrTokenInvalid
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, ErrTokenInvalid
	}

	return userID, nil
}

// validateEmail checks whether the email is a valid format.
func validateEmail(email string) error {
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return ErrInvalidEmail
	}

	parts := strings.Split(addr.Address, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || !strings.Contains(parts[1], ".") {
		return ErrInvalidEmail
	}

	return nil
}
