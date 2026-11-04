package link

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/httpx"
)

const maxCodeAttempts = 5

// reservedCodes are URL prefixes used by the API and health endpoints.
var reservedCodes = map[string]bool{
	"v1":      true,
	"healthz": true,
	"readyz":  true,
}

// customCodeRe matches valid custom codes: 4-32 chars from [A-Za-z0-9_-].
var customCodeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{4,32}$`)

// CreateInput holds the data needed to create a new link.
type CreateInput struct {
	UserID     uuid.UUID
	URL        string
	CustomCode string
	ExpiresAt  *time.Time
}

// Service implements link management business logic.
type Service struct {
	repo Repository
	gen  CodeGenerator
	now  func() time.Time
}

// NewService returns a new Service.
func NewService(repo Repository, gen CodeGenerator, now func() time.Time) *Service {
	return &Service{repo: repo, gen: gen, now: now}
}

// Create validates the input and persists a new link.
// For random codes it retries up to maxCodeAttempts on collision.
func (s *Service) Create(ctx context.Context, in CreateInput) (Link, error) {
	if err := validateTarget(in.URL); err != nil {
		return Link{}, err
	}

	if in.ExpiresAt != nil && !in.ExpiresAt.After(s.now()) {
		return Link{}, ErrInvalidExpiry
	}

	l := Link{
		ID:        uuid.New(),
		UserID:    in.UserID,
		TargetURL: in.URL,
		ExpiresAt: in.ExpiresAt,
		CreatedAt: s.now(),
	}

	if in.CustomCode != "" {
		if err := validateCustomCode(in.CustomCode); err != nil {
			return Link{}, err
		}
		l.Code = in.CustomCode
		if err := s.repo.Insert(ctx, l); err != nil {
			return Link{}, err // ErrCodeTaken passes through unchanged
		}
		return l, nil
	}

	for range maxCodeAttempts {
		code, err := s.gen.New()
		if err != nil {
			return Link{}, fmt.Errorf("generate code: %w", err)
		}
		l.Code = code
		err = s.repo.Insert(ctx, l)
		if errors.Is(err, ErrCodeTaken) {
			continue
		}
		if err != nil {
			return Link{}, fmt.Errorf("insert link: %w", err)
		}
		return l, nil
	}

	return Link{}, ErrCodeExhausted
}

// Get returns a link owned by userID. Returns ErrNotFound for unknown codes or other owners.
func (s *Service) Get(ctx context.Context, userID uuid.UUID, code string) (Link, error) {
	l, err := s.repo.GetOwned(ctx, code, userID)
	if err != nil {
		return Link{}, err
	}
	return l, nil
}

// Resolve returns a link by code for redirect, checking expiry.
// Returns ErrNotFound for missing codes, ErrExpired for expired links.
func (s *Service) Resolve(ctx context.Context, code string) (Link, error) {
	l, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return Link{}, err
	}

	if l.ExpiresAt != nil && !s.now().Before(*l.ExpiresAt) {
		return Link{}, ErrExpired
	}

	return l, nil
}

// List returns a page of links for the owner, using keyset pagination.
// limit is clamped to [1, 100]. Returns the next cursor when there are more results.
func (s *Service) List(ctx context.Context, userID uuid.UUID, limit int, cursor *httpx.Cursor) ([]Link, *httpx.Cursor, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var after *Cursor
	if cursor != nil {
		after = &Cursor{CreatedAt: cursor.CreatedAt, ID: cursor.ID}
	}

	// Fetch one extra to determine whether a next page exists.
	rows, err := s.repo.ListByUser(ctx, userID, after, limit+1)
	if err != nil {
		return nil, nil, fmt.Errorf("list links: %w", err)
	}

	var next *httpx.Cursor
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = &httpx.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}

	return rows, next, nil
}

// Delete removes a link owned by userID.
func (s *Service) Delete(ctx context.Context, userID uuid.UUID, code string) error {
	return s.repo.DeleteOwned(ctx, code, userID)
}

// validateTarget checks that u is a valid http or https URL.
func validateTarget(u string) error {
	if len(u) > 2048 {
		return ErrInvalidURL
	}

	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		return ErrInvalidURL
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ErrInvalidURL
	}

	if parsed.User != nil {
		return ErrInvalidURL
	}

	return nil
}

// validateCustomCode checks length, charset, and reserved names.
func validateCustomCode(code string) error {
	if !customCodeRe.MatchString(code) {
		return ErrInvalidCode
	}
	if reservedCodes[strings.ToLower(code)] {
		return ErrInvalidCode
	}
	return nil
}
